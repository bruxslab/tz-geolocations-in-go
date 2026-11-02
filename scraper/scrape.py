#!/usr/bin/env python3
"""Async scraper for tanzaniapostcode.com — sitemap-based extraction.

Instead of scraping thousands of ward/street pages (which are bot-protected),
this approach extracts all 75,145 streets from the sitemap XML files where
street URLs encode the full hierarchy:

    https://www.tanzaniapostcode.com/arusha-arumeru-akheri-duluti-23306.html
                                      ^region ^district ^ward ^street ^postcode

Only ~35 HTTP requests needed (homepage + 31 region pages + 3 sitemaps).

Usage:
  python scrape.py              # Full scrape
  python scrape.py --resume     # Resume from last checkpoint
"""

import asyncio
import gzip
import json
import logging
import random
import re
import sys
import time
from collections import Counter, defaultdict
from pathlib import Path

import httpx

from parse import BASE_URL, parse_regions, parse_child_links

# --- Configuration ---
MAX_CONCURRENT = 3
DELAY_MIN = 1.5
DELAY_MAX = 3.0
TIMEOUT = 30.0
MAX_RETRIES = 5
BACKOFF_BASE = 4
EXPECTED_TOTAL_STREETS = 75061

SITEMAP_BASE = f"{BASE_URL}/xml"
LISTING_SITEMAPS = ["listing_part1.xml.gz", "listing_part2.xml.gz"]
LOCATIONS_SITEMAP = "locations.xml.gz"

HEADERS = {
    "User-Agent": (
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
    ),
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
    "Accept-Language": "en-US,en;q=0.9",
    "Accept-Encoding": "gzip, deflate",
    "Connection": "keep-alive",
    "Upgrade-Insecure-Requests": "1",
    "Sec-Fetch-Dest": "document",
    "Sec-Fetch-Mode": "navigate",
    "Sec-Fetch-Site": "same-origin",
    "Sec-Fetch-User": "?1",
    "Cache-Control": "max-age=0",
}

BOT_BLOCK_SIGNATURES = [
    "Unusual Traffic Activity",
    "unusual traffic",
    "Access Denied",
    "captcha",
]

DATA_DIR = Path(__file__).parent / "data"
DATA_DIR.mkdir(exist_ok=True)

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
log = logging.getLogger("scraper")

# --- Helpers ---

semaphore: asyncio.Semaphore


def is_bot_blocked(html: str) -> bool:
    """Check if the response HTML is a bot-block page."""
    lower = html.lower()
    return any(sig.lower() in lower for sig in BOT_BLOCK_SIGNATURES)


async def fetch(client: httpx.AsyncClient, url: str) -> str | None:
    """Fetch a URL with rate limiting, bot-block detection, and retries."""
    async with semaphore:
        for attempt in range(1, MAX_RETRIES + 1):
            try:
                await asyncio.sleep(random.uniform(DELAY_MIN, DELAY_MAX))

                # Randomize Referer header (60% include, 40% omit)
                headers = {}
                if random.random() < 0.6:
                    headers["Referer"] = f"{BASE_URL}/"

                resp = await client.get(url, headers=headers)
                resp.raise_for_status()
                html = resp.text

                if is_bot_blocked(html):
                    backoff = BACKOFF_BASE * (2 ** (attempt - 1)) + random.uniform(0, 2)
                    log.warning(
                        f"Bot block detected for {url} (attempt {attempt}), "
                        f"backing off {backoff:.1f}s"
                    )
                    if attempt == MAX_RETRIES:
                        log.error(f"Still bot-blocked after {MAX_RETRIES} attempts: {url}")
                        return None
                    await asyncio.sleep(backoff)
                    continue

                return html
            except httpx.HTTPStatusError as e:
                log.warning(f"HTTP {e.response.status_code} for {url} (attempt {attempt})")
                if attempt == MAX_RETRIES:
                    log.error(f"Failed after {MAX_RETRIES} attempts: {url}")
                    return None
                await asyncio.sleep(2 ** attempt)
            except httpx.RequestError as e:
                log.warning(f"Request error for {url}: {e} (attempt {attempt})")
                if attempt == MAX_RETRIES:
                    log.error(f"Failed after {MAX_RETRIES} attempts: {url}")
                    return None
                await asyncio.sleep(2 ** attempt)
    return None


async def fetch_sitemap_xml(client: httpx.AsyncClient, name: str) -> str:
    """Download and decompress a .xml.gz sitemap file."""
    url = f"{SITEMAP_BASE}/{name}"
    log.info(f"Downloading sitemap: {url}")
    for attempt in range(1, MAX_RETRIES + 1):
        try:
            resp = await client.get(url)
            resp.raise_for_status()
            # The response content is gzip-compressed XML
            xml_bytes = gzip.decompress(resp.content)
            xml_text = xml_bytes.decode("utf-8")
            log.info(f"Sitemap {name}: {len(xml_text)} bytes decompressed")
            return xml_text
        except Exception as e:
            log.warning(f"Failed to fetch sitemap {name} (attempt {attempt}): {e}")
            if attempt == MAX_RETRIES:
                raise RuntimeError(f"Failed to download sitemap {name} after {MAX_RETRIES} attempts")
            await asyncio.sleep(2 ** attempt)
    return ""


def parse_sitemap_urls(xml: str) -> list[str]:
    """Extract all <loc> URLs from sitemap XML."""
    return re.findall(r"<loc>\s*(.*?)\s*</loc>", xml)


def slug_to_name(slug: str) -> str:
    """Convert a URL slug to a Title Case name.

    Examples:
        'king-ori' -> 'King Ori'
        'dar-es-salaam' -> 'Dar Es Salaam'
        'nyang-hwale' -> 'Nyang Hwale'
    """
    return slug.replace("-", " ").title()


def save_checkpoint(name: str, data: object) -> None:
    """Save intermediate data as a checkpoint file."""
    path = DATA_DIR / f"{name}.checkpoint.json"
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2))
    log.info(f"Checkpoint saved: {path}")


def load_checkpoint(name: str) -> object | None:
    """Load a checkpoint file if it exists."""
    path = DATA_DIR / f"{name}.checkpoint.json"
    if path.exists():
        log.info(f"Loading checkpoint: {path}")
        return json.loads(path.read_text())
    return None


def save_output(name: str, data: object) -> None:
    """Save final output JSON."""
    path = DATA_DIR / f"{name}.json"
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2))
    log.info(f"Output saved: {path} ({len(data) if isinstance(data, list) else 'N/A'} items)")


# --- Step 1: Fetch Regions ---

async def scrape_regions(client: httpx.AsyncClient, resume: bool) -> list[dict]:
    """Fetch the /location/ page and parse the 31 regions."""
    if resume:
        cached = load_checkpoint("regions")
        if cached:
            return cached

    log.info("Scraping regions...")
    html = await fetch(client, f"{BASE_URL}/location/")
    if not html:
        raise RuntimeError("Failed to fetch regions page")

    regions = parse_regions(html)
    log.info(f"Found {len(regions)} regions")

    if len(regions) != 31:
        log.warning(f"Expected 31 regions, got {len(regions)}")

    total_expected = sum(r.get("expected_count", 0) for r in regions)
    log.info(f"Total expected streets from region counts: {total_expected}")

    save_checkpoint("regions", regions)
    return regions


# --- Step 2: Fetch Districts from Region Pages ---

async def scrape_districts(
    client: httpx.AsyncClient, regions: list[dict], resume: bool
) -> list[dict]:
    """Fetch each region page to get district names and slugs."""
    if resume:
        cached = load_checkpoint("districts")
        if cached:
            return cached

    log.info("Scraping districts from region pages...")
    all_districts = []

    async def fetch_region_districts(region: dict) -> list[dict]:
        html = await fetch(client, region["url"])
        if not html:
            log.warning(f"Failed to fetch districts for {region['name']}")
            return []
        districts = parse_child_links(html, expected_depth=2)
        for d in districts:
            d["region_slug"] = region["slug"]
            d["region_name"] = region["name"]
        return districts

    tasks = [fetch_region_districts(r) for r in regions]
    results = await asyncio.gather(*tasks)
    for district_list in results:
        all_districts.extend(district_list)

    # Verify every region produced at least one district
    region_district_counts = Counter(d["region_slug"] for d in all_districts)
    for r in regions:
        if r["slug"] not in region_district_counts:
            log.error(f"Region '{r['name']}' has NO districts!")

    log.info(f"Found {len(all_districts)} districts across {len(regions)} regions")
    save_checkpoint("districts", all_districts)
    return all_districts


# --- Step 3: Download and Parse Sitemaps ---

async def download_sitemaps(
    client: httpx.AsyncClient, resume: bool
) -> tuple[list[str], list[str]]:
    """Download locations.xml.gz and listing sitemaps, return parsed URLs."""
    if resume:
        cached = load_checkpoint("sitemaps")
        if cached:
            return cached["location_urls"], cached["listing_urls"]

    log.info("Downloading sitemaps...")

    # Fetch all three sitemaps
    locations_xml = await fetch_sitemap_xml(client, LOCATIONS_SITEMAP)
    location_urls = parse_sitemap_urls(locations_xml)
    log.info(f"Locations sitemap: {len(location_urls)} URLs")

    listing_urls = []
    for sitemap_name in LISTING_SITEMAPS:
        xml = await fetch_sitemap_xml(client, sitemap_name)
        urls = parse_sitemap_urls(xml)
        log.info(f"{sitemap_name}: {len(urls)} URLs")
        listing_urls.extend(urls)

    log.info(f"Total listing URLs: {len(listing_urls)}")

    save_checkpoint("sitemaps", {
        "location_urls": location_urls,
        "listing_urls": listing_urls,
    })
    return location_urls, listing_urls


# --- Step 4 & 5: Parse Listing URLs and Discover Hierarchy ---

def _hyphen_lcp(strings: list[str]) -> str:
    """Compute longest common prefix at hyphen-boundaries for a list of strings.

    Example:
        ['arusha-arumeru-akheri-duluti', 'arusha-arumeru-akheri-kibiriti']
        -> 'arusha-arumeru-akheri'
    """
    if not strings:
        return ""
    if len(strings) == 1:
        # For a single string, the LCP is everything except the last segment
        # (last segment = street name)
        parts = strings[0].split("-")
        if len(parts) > 1:
            return "-".join(parts[:-1])
        return strings[0]

    # Split all strings into hyphen-delimited parts
    split = [s.split("-") for s in strings]
    min_len = min(len(parts) for parts in split)

    prefix_parts = []
    for i in range(min_len):
        segment = split[0][i]
        if all(parts[i] == segment for parts in split):
            prefix_parts.append(segment)
        else:
            break

    return "-".join(prefix_parts)


def parse_listing_url(url: str) -> tuple[str, str] | None:
    """Parse a listing URL into (body, postcode).

    URL format: https://www.tanzaniapostcode.com/{body}-{postcode}.html
    Returns (body, postcode) or None if invalid.
    """
    # Extract filename from URL
    filename = url.rstrip("/").rsplit("/", 1)[-1]
    if not filename.endswith(".html"):
        return None
    filename = filename[:-5]  # strip .html

    # Postcode is always the last 5 digits after the final hyphen
    parts = filename.rsplit("-", 1)
    if len(parts) != 2:
        return None

    body, postcode = parts
    if not postcode.isdigit() or len(postcode) != 5:
        return None

    return body, postcode


def build_known_wards_from_locations(location_urls: list[str]) -> dict[str, set[str]]:
    """Parse locations.xml.gz URLs to build known (region, district, ward) triples.

    Location URLs have format:
        /location/{region}/
        /location/{region}/{district}/
        /location/{region}/{district}/{ward}/

    Returns dict mapping (region_slug, district_slug) -> set of ward_slugs.
    """
    ward_map: dict[tuple[str, str], set[str]] = defaultdict(set)
    district_slugs_by_region: dict[str, set[str]] = defaultdict(set)

    for url in location_urls:
        path = url.rstrip("/")
        if "/location/" not in path:
            continue
        after = path.split("/location/", 1)[1]
        if not after:
            continue
        segments = after.split("/")

        if len(segments) == 2:
            # District URL
            region_slug, district_slug = segments
            district_slugs_by_region[region_slug].add(district_slug)
        elif len(segments) == 3:
            # Ward URL
            region_slug, district_slug, ward_slug = segments
            ward_map[(region_slug, district_slug)].add(ward_slug)
            district_slugs_by_region[region_slug].add(district_slug)

    return ward_map, district_slugs_by_region


def discover_hierarchy(
    listing_urls: list[str],
    regions: list[dict],
    known_districts: list[dict],
    location_urls: list[str],
) -> tuple[list[dict], list[dict], list[dict]]:
    """Core algorithm: parse listing URLs to discover wards and streets.

    Uses the LCP (Longest Common Prefix) approach to determine ward boundaries
    from street URL patterns grouped by (region, postcode).

    Returns (districts, wards, streets) with full hierarchy.
    """
    # Build lookup structures
    region_slugs = {r["slug"] for r in regions}
    region_slug_list = sorted(region_slugs, key=len, reverse=True)  # longest first for matching

    region_name_map = {r["slug"]: r["name"] for r in regions}

    # Known districts from region pages: {region_slug: {district_slug: name}}
    known_district_map: dict[str, dict[str, str]] = defaultdict(dict)
    for d in known_districts:
        known_district_map[d["region_slug"]][d["slug"]] = d["name"]

    # Known wards from locations.xml.gz
    known_ward_map, sitemap_district_slugs = build_known_wards_from_locations(location_urls)

    # Merge sitemap district slugs into known_district_map (without overwriting names)
    for region_slug, district_slugs in sitemap_district_slugs.items():
        for ds in district_slugs:
            if ds not in known_district_map.get(region_slug, {}):
                if region_slug not in known_district_map:
                    known_district_map[region_slug] = {}
                known_district_map[region_slug][ds] = slug_to_name(ds)

    # Build set of all known district slugs per region (for matching)
    all_district_slugs: dict[str, set[str]] = defaultdict(set)
    for region_slug, districts in known_district_map.items():
        all_district_slugs[region_slug] = set(districts.keys())

    # Step 4: Parse all listing URLs
    parsed_entries = []  # (body, postcode, region_slug)
    unparsed = 0

    for url in listing_urls:
        result = parse_listing_url(url)
        if result is None:
            unparsed += 1
            continue

        body, postcode = result

        # Match region slug at start of body
        matched_region = None
        for rs in region_slug_list:
            if body.startswith(rs + "-"):
                matched_region = rs
                break

        if matched_region is None:
            unparsed += 1
            continue

        parsed_entries.append((body, postcode, matched_region))

    log.info(f"Parsed {len(parsed_entries)} listing URLs ({unparsed} unparsed)")

    # Group by (region, postcode) — each group = one ward
    groups: dict[tuple[str, str], list[str]] = defaultdict(list)
    for body, postcode, region_slug in parsed_entries:
        groups[(region_slug, postcode)].append(body)

    log.info(f"Found {len(groups)} (region, postcode) groups (= ward candidates)")

    # Step 5: Discover ward hierarchy via LCP
    # For each group, compute LCP to find region-district-ward prefix
    discovered_districts: dict[str, dict[str, str]] = defaultdict(dict)  # region -> {slug: name}
    ward_records = []
    street_records = []

    for (region_slug, postcode), bodies in groups.items():
        # Compute LCP of all bodies in this group
        lcp = _hyphen_lcp(bodies)

        # Remove region prefix from LCP
        region_prefix = region_slug + "-"
        if lcp.startswith(region_prefix):
            after_region = lcp[len(region_prefix):]
        else:
            # Shouldn't happen, but handle gracefully
            log.warning(f"LCP '{lcp}' doesn't start with region '{region_slug}'")
            after_region = lcp

        # Split after_region into district_slug and ward_slug
        # Try to match known district slugs (longest first)
        district_slug = None
        ward_slug = None

        known_ds = all_district_slugs.get(region_slug, set())
        # Sort by length descending to match longest district slug first
        sorted_known = sorted(known_ds, key=len, reverse=True)

        for ds in sorted_known:
            if after_region == ds:
                # Edge case: LCP is exactly the district slug (ward = district name)
                district_slug = ds
                ward_slug = ds
                break
            elif after_region.startswith(ds + "-"):
                district_slug = ds
                ward_slug = after_region[len(ds) + 1:]
                break

        if district_slug is None:
            # Unknown district — try to discover it
            # The after_region format is "district-ward" but we don't know the split point
            # We'll collect these and resolve them in a second pass
            # For now, store the full after_region
            district_slug = "__unknown__"
            ward_slug = after_region

        # Extract streets from this group
        ward_prefix = lcp
        for body in bodies:
            if body == ward_prefix:
                # The street name is the same as the ward (single-word streets)
                street_slug = ward_slug.split("-")[-1] if ward_slug else ""
            elif body.startswith(ward_prefix + "-"):
                street_slug = body[len(ward_prefix) + 1:]
            else:
                # Body doesn't start with LCP — use the part after region-district-ward
                street_slug = body

            street_records.append({
                "street_slug": street_slug,
                "name": slug_to_name(street_slug) if street_slug else "",
                "postcode": postcode,
                "ward_slug": ward_slug,
                "district_slug": district_slug,
                "region_slug": region_slug,
                "_body": body,
                "_ward_prefix": ward_prefix,
            })

        ward_records.append({
            "slug": ward_slug,
            "district_slug": district_slug,
            "region_slug": region_slug,
            "postcode": postcode,
            "_after_region": after_region,
        })

    # Second pass: resolve unknown districts
    # Group unknown ward records by region and find common district prefix
    unknown_by_region: dict[str, list[dict]] = defaultdict(list)
    for wr in ward_records:
        if wr["district_slug"] == "__unknown__":
            unknown_by_region[wr["region_slug"]].append(wr)

    for region_slug, unknown_wards in unknown_by_region.items():
        # Get all after_region strings for this region's unknown wards
        after_regions = [wr["_after_region"] for wr in unknown_wards]

        # Find potential district prefixes by grouping
        # Try to find the longest common prefix among subgroups
        # that share the same district
        prefix_groups = _discover_district_prefixes(after_regions, region_slug)

        for district_prefix, ward_suffixes in prefix_groups.items():
            discovered_districts[region_slug][district_prefix] = slug_to_name(district_prefix)
            all_district_slugs[region_slug].add(district_prefix)
            log.info(
                f"Discovered phantom district: {region_slug}/{district_prefix} "
                f"({len(ward_suffixes)} wards)"
            )

            # Update ward and street records
            for wr in unknown_wards:
                after_region = wr["_after_region"]
                if after_region.startswith(district_prefix + "-"):
                    new_ward_slug = after_region[len(district_prefix) + 1:]
                    wr["district_slug"] = district_prefix
                    wr["slug"] = new_ward_slug
                elif after_region == district_prefix:
                    wr["district_slug"] = district_prefix
                    wr["slug"] = district_prefix

            # Update street records too
            for sr in street_records:
                if sr["region_slug"] == region_slug and sr["district_slug"] == "__unknown__":
                    after_region = sr["_body"]
                    # Remove region prefix
                    ar = after_region[len(region_slug) + 1:] if after_region.startswith(region_slug + "-") else after_region
                    if ar.startswith(district_prefix + "-"):
                        sr["district_slug"] = district_prefix
                        # Recompute ward slug
                        remainder = ar[len(district_prefix) + 1:]
                        # The ward slug for this street's group
                        for wr in unknown_wards:
                            if wr["_after_region"] == ar[:len(wr["_after_region"])] or ar.startswith(wr["_after_region"]):
                                sr["ward_slug"] = wr["slug"]
                                break

    # Handle any still-unknown entries (fallback: use first two segments as district)
    still_unknown = [wr for wr in ward_records if wr["district_slug"] == "__unknown__"]
    if still_unknown:
        log.warning(f"{len(still_unknown)} ward records still have unknown district")
        for wr in still_unknown:
            after_region = wr["_after_region"]
            parts = after_region.split("-")
            if len(parts) >= 2:
                wr["district_slug"] = parts[0]
                wr["slug"] = "-".join(parts[1:])
                discovered_districts[wr["region_slug"]][parts[0]] = slug_to_name(parts[0])
            else:
                wr["district_slug"] = after_region
                wr["slug"] = after_region

        # Also fix street records
        for sr in street_records:
            if sr["district_slug"] == "__unknown__":
                ar = sr["_body"]
                if ar.startswith(sr["region_slug"] + "-"):
                    ar = ar[len(sr["region_slug"]) + 1:]
                parts = ar.split("-")
                if len(parts) >= 2:
                    sr["district_slug"] = parts[0]

    return discovered_districts, ward_records, street_records


def _discover_district_prefixes(
    after_regions: list[str], region_slug: str
) -> dict[str, list[str]]:
    """Discover district prefixes from a list of after-region strings.

    When multiple ward groups share a common prefix, that prefix is the district.

    Example:
        ['nyang-hwale-ward1', 'nyang-hwale-ward2'] -> {'nyang-hwale': ['ward1', 'ward2']}
    """
    if not after_regions:
        return {}

    # Try to find shared prefixes among the after_region strings
    # Split each into hyphen-segments and look for groupings
    result: dict[str, list[str]] = defaultdict(list)

    # Compute LCP of all strings at hyphen boundaries
    overall_lcp = _hyphen_lcp(after_regions)

    if overall_lcp and overall_lcp not in [ar for ar in after_regions]:
        # The overall LCP is the district prefix
        for ar in after_regions:
            if ar.startswith(overall_lcp + "-"):
                suffix = ar[len(overall_lcp) + 1:]
                result[overall_lcp].append(suffix)
            elif ar == overall_lcp:
                result[overall_lcp].append(overall_lcp)
        if result:
            return dict(result)

    # If no shared LCP, try to find sub-groups
    # Group by first N segments and see which grouping makes sense
    # Use first-2-segments as candidate district prefix
    candidate_groups: dict[str, list[str]] = defaultdict(list)
    for ar in after_regions:
        parts = ar.split("-")
        # Try progressively shorter prefixes
        for n in range(min(3, len(parts) - 1), 0, -1):
            candidate = "-".join(parts[:n])
            rest = "-".join(parts[n:])
            candidate_groups[candidate].append(rest)
            break  # use longest candidate that leaves a remainder

    # A valid district prefix should have multiple wards
    for prefix, suffixes in candidate_groups.items():
        if len(suffixes) >= 1:
            result[prefix] = suffixes

    return dict(result)


def resolve_hierarchy(
    regions: list[dict],
    scraped_districts: list[dict],
    discovered_districts: dict[str, dict[str, str]],
    ward_records: list[dict],
    street_records: list[dict],
) -> tuple[list[dict], list[dict], list[dict]]:
    """Merge all data sources and build clean district, ward, and street lists."""
    region_name_map = {r["slug"]: r["name"] for r in regions}

    # Build final districts list
    # Start with scraped districts (from region pages)
    district_set: dict[tuple[str, str], dict] = {}
    for d in scraped_districts:
        key = (d["region_slug"], d["slug"])
        district_set[key] = {
            "name": d["name"],
            "slug": d["slug"],
            "region_slug": d["region_slug"],
            "region_name": d.get("region_name", region_name_map.get(d["region_slug"], "")),
        }

    # Add discovered (phantom) districts
    for region_slug, districts in discovered_districts.items():
        for slug, name in districts.items():
            key = (region_slug, slug)
            if key not in district_set:
                district_set[key] = {
                    "name": name,
                    "slug": slug,
                    "region_slug": region_slug,
                    "region_name": region_name_map.get(region_slug, ""),
                }
                log.info(f"Added phantom district: {region_slug}/{slug} ({name})")

    # Also add districts that appear in ward records but not yet in district_set
    for wr in ward_records:
        key = (wr["region_slug"], wr["district_slug"])
        if key not in district_set:
            district_set[key] = {
                "name": slug_to_name(wr["district_slug"]),
                "slug": wr["district_slug"],
                "region_slug": wr["region_slug"],
                "region_name": region_name_map.get(wr["region_slug"], ""),
            }

    final_districts = sorted(district_set.values(), key=lambda d: (d["region_slug"], d["slug"]))

    # Build final wards list (deduplicated)
    ward_set: dict[tuple[str, str, str], dict] = {}
    for wr in ward_records:
        key = (wr["region_slug"], wr["district_slug"], wr["slug"])
        if key not in ward_set:
            ward_set[key] = {
                "name": slug_to_name(wr["slug"]),
                "slug": wr["slug"],
                "district_slug": wr["district_slug"],
                "region_slug": wr["region_slug"],
                "postcode": wr["postcode"],
            }

    final_wards = sorted(ward_set.values(), key=lambda w: (w["region_slug"], w["district_slug"], w["slug"]))

    # Build final streets list (deduplicated — sitemaps contain duplicate URLs)
    final_streets = []
    seen_streets: set[tuple[str, str, str, str]] = set()
    dupes_removed = 0
    for sr in street_records:
        key = (sr["region_slug"], sr["district_slug"], sr["ward_slug"], sr["name"])
        if key in seen_streets:
            dupes_removed += 1
            continue
        seen_streets.add(key)
        final_streets.append({
            "name": sr["name"],
            "postcode": sr["postcode"],
            "ward_slug": sr["ward_slug"],
            "district_slug": sr["district_slug"],
            "region_slug": sr["region_slug"],
        })
    if dupes_removed:
        log.info(f"Removed {dupes_removed} duplicate street entries from sitemap data")

    # Sort streets for deterministic output
    final_streets.sort(key=lambda s: (s["region_slug"], s["district_slug"], s["ward_slug"], s["name"]))

    return final_districts, final_wards, final_streets


# --- Validation ---

def validate(regions, districts, wards, streets):
    """Validate scraped data."""
    errors = []

    # Region count
    if len(regions) != 31:
        errors.append(f"Expected exactly 31 regions, got {len(regions)}")

    # Total streets
    if len(streets) != EXPECTED_TOTAL_STREETS:
        errors.append(
            f"Expected {EXPECTED_TOTAL_STREETS} streets, got {len(streets)}"
        )

    # Per-region street counts vs site-displayed counts
    # (site counts include duplicate sitemap entries, so minor differences are expected)
    region_street_counts = Counter(s["region_slug"] for s in streets)
    for r in regions:
        site_count = r.get("expected_count", 0)
        actual = region_street_counts.get(r["slug"], 0)
        if site_count and actual > site_count:
            errors.append(
                f"Region '{r['name']}': has MORE streets ({actual}) than site claims ({site_count})"
            )
        elif site_count and actual < site_count:
            log.info(
                f"Region '{r['name']}': {actual} streets (site shows {site_count}, "
                f"diff={site_count - actual} dupes removed)"
            )

    # Every region has at least one district
    region_district_counts = Counter(d["region_slug"] for d in districts)
    for r in regions:
        if r["slug"] not in region_district_counts:
            errors.append(f"Region '{r['name']}' has no districts")

    # Every district has at least one ward
    district_ward_counts = Counter(
        f"{w['region_slug']}/{w['district_slug']}" for w in wards
    )
    for d in districts:
        key = f"{d['region_slug']}/{d['slug']}"
        if key not in district_ward_counts:
            errors.append(f"District '{d['name']}' ({d['region_slug']}) has no wards")

    # Every ward has at least one street
    ward_street_counts = Counter(
        f"{s['region_slug']}/{s['district_slug']}/{s['ward_slug']}" for s in streets
    )
    for w in wards:
        key = f"{w['region_slug']}/{w['district_slug']}/{w['slug']}"
        if key not in ward_street_counts:
            errors.append(f"Ward '{w['name']}' ({w['district_slug']}/{w['region_slug']}) has no streets")

    # Check postcodes are 5-digit strings
    bad_postcodes = 0
    for s in streets:
        pc = s.get("postcode", "")
        if not pc or not pc.isdigit() or len(pc) != 5:
            bad_postcodes += 1
    if bad_postcodes > 0:
        errors.append(f"{bad_postcodes} streets have missing or invalid postcodes")

    # Ward postcodes
    wards_without_postcode = sum(1 for w in wards if not w.get("postcode"))
    if wards_without_postcode > 0:
        errors.append(f"{wards_without_postcode} wards missing postcode")

    # Summary
    log.info("=== Validation Summary ===")
    log.info(f"Regions:   {len(regions)}")
    log.info(f"Districts: {len(districts)}")
    log.info(f"Wards:     {len(wards)}")
    log.info(f"Streets:   {len(streets)}")

    if errors:
        for e in errors:
            log.warning(f"VALIDATION: {e}")
    else:
        log.info("All validations passed!")

    return len(errors) == 0


# --- Build Output Files ---

def build_outputs(regions, districts, wards, streets):
    """Build and save final output JSON files."""
    regions_out = [{"name": r["name"], "slug": r["slug"]} for r in regions]

    districts_out = [
        {
            "name": d["name"],
            "slug": d["slug"],
            "region_slug": d["region_slug"],
        }
        for d in districts
    ]

    wards_out = []
    wards_missing_pc = 0
    for w in wards:
        postcode = w.get("postcode", "")
        if not postcode:
            wards_missing_pc += 1
            log.warning(f"Ward '{w['name']}' ({w['district_slug']}/{w['region_slug']}) has no postcode in output")
        wards_out.append({
            "name": w["name"],
            "slug": w["slug"],
            "district_slug": w["district_slug"],
            "region_slug": w["region_slug"],
            "postcode": postcode,
        })
    if wards_missing_pc:
        log.warning(f"{wards_missing_pc} wards written without postcode")

    streets_out = [
        {
            "name": s["name"],
            "postcode": s.get("postcode", ""),
            "ward_slug": s["ward_slug"],
            "district_slug": s["district_slug"],
            "region_slug": s["region_slug"],
        }
        for s in streets
    ]

    # Denormalized locations.json
    locations = []
    for r in regions_out:
        locations.append({"type": "region", **r})
    for d in districts_out:
        locations.append({"type": "district", **d})
    for w in wards_out:
        locations.append({"type": "ward", **w})
    for s in streets_out:
        locations.append({"type": "street", **s})

    save_output("regions", regions_out)
    save_output("districts", districts_out)
    save_output("wards", wards_out)
    save_output("streets", streets_out)
    save_output("locations", locations)


# --- Main ---

async def main():
    resume = "--resume" in sys.argv

    global semaphore
    semaphore = asyncio.Semaphore(MAX_CONCURRENT)

    start_time = time.time()

    async with httpx.AsyncClient(
        headers=HEADERS,
        timeout=httpx.Timeout(TIMEOUT),
        http2=True,
        follow_redirects=True,
    ) as client:
        # Warm up session: fetch homepage to collect cookies
        log.info("Warming up session (fetching homepage)...")
        await client.get(f"{BASE_URL}/")
        await asyncio.sleep(random.uniform(3, 6))

        # Step 1: Fetch regions
        regions = await scrape_regions(client, resume)

        # Step 2: Fetch districts from region pages
        scraped_districts = await scrape_districts(client, regions, resume)

        # Step 3: Download sitemaps
        location_urls, listing_urls = await download_sitemaps(client, resume)

    elapsed_fetch = time.time() - start_time
    log.info(f"Fetching completed in {elapsed_fetch:.1f}s")

    # Steps 4-5: Parse listing URLs and discover hierarchy
    log.info("Discovering hierarchy from listing URLs...")
    discovered_districts, ward_records, street_records = discover_hierarchy(
        listing_urls, regions, scraped_districts, location_urls
    )

    # Step 6: Resolve and enrich hierarchy
    log.info("Resolving hierarchy...")
    districts, wards, streets = resolve_hierarchy(
        regions, scraped_districts, discovered_districts, ward_records, street_records
    )

    elapsed = time.time() - start_time
    log.info(f"Total processing completed in {elapsed:.1f}s")

    # Step 7: Validate and output
    validate(regions, districts, wards, streets)
    build_outputs(regions, districts, wards, streets)


if __name__ == "__main__":
    asyncio.run(main())
