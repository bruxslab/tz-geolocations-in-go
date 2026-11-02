"""HTML parsing helpers for tanzaniapostcode.com."""

import re

from bs4 import BeautifulSoup, Tag


BASE_URL = "https://www.tanzaniapostcode.com"


def _count_segments(href: str) -> int:
    """Count path segments after /location/ in a href.

    /location/arusha/          → 1
    /location/arusha/arumeru/  → 2
    /location/arusha/arumeru/akheri/ → 3
    """
    path = href.strip().rstrip("/")
    prefix = "/location/"
    idx = path.find(prefix)
    if idx == -1:
        return 0
    after = path[idx + len(prefix) :]
    if not after:
        return 0
    return len(after.split("/"))


def _extract_count(text: str) -> int:
    """Extract the location count from text like 'Region Name (1865)'.

    Returns 0 if no count found.
    """
    match = re.search(r"\((\d+)\)", text)
    return int(match.group(1)) if match else 0


def parse_regions(html: str) -> list[dict]:
    """Parse the /location/ page to extract the 31 regions.

    Regions are listed as <h4> headings containing <a href="/location/{slug}/">.
    Also extracts expected_count from the (N) shown after each region name.
    """
    soup = BeautifulSoup(html, "lxml")
    results = []
    seen = set()

    for h4 in soup.find_all("h4"):
        link = h4.find("a", href=True)
        if not link:
            continue
        href = link["href"]
        if "/location/" not in href:
            continue
        if _count_segments(href) != 1:
            continue

        name = link.get_text(strip=True)
        slug = href.rstrip("/").split("/")[-1]
        if not name or not slug or slug in seen:
            continue

        seen.add(slug)
        url = href if href.startswith("http") else BASE_URL + href
        expected_count = _extract_count(h4.get_text())
        results.append({
            "name": name,
            "slug": slug,
            "url": url,
            "expected_count": expected_count,
        })

    return results


def parse_child_links(html: str, expected_depth: int) -> list[dict]:
    """Parse a region or district page to extract child location links.

    Args:
        html: Page HTML.
        expected_depth: Number of path segments expected after /location/.
            2 for districts (/location/{region}/{district}/)
            3 for wards (/location/{region}/{district}/{ward}/)

    Filters out breadcrumb/parent links and any links at the wrong depth.
    Also extracts expected_count from (N) in the parent heading element.
    """
    soup = BeautifulSoup(html, "lxml")
    results = []
    seen = set()

    for link in soup.select("a[href*='/location/']"):
        href = link.get("href", "")
        if _count_segments(href) != expected_depth:
            continue

        name = link.get_text(strip=True)
        slug = href.rstrip("/").split("/")[-1]
        if not name or not slug or slug in seen:
            continue

        seen.add(slug)
        url = href if href.startswith("http") else BASE_URL + href

        # Extract count from parent heading (h4 or similar)
        parent = link.find_parent(["h4", "h3", "h5"])
        expected_count = _extract_count(parent.get_text()) if parent else 0

        results.append({
            "name": name,
            "slug": slug,
            "url": url,
            "expected_count": expected_count,
        })

    return results


def parse_street_table(html: str) -> list[dict[str, str]]:
    """Parse a ward page to extract all 5 columns from the street table.

    Table columns: Location | Ward | District | Region | Postcode
    Returns list of dicts with 'name', 'ward', 'district', 'region', 'postcode'.
    """
    soup = BeautifulSoup(html, "lxml")
    results = []

    table = soup.find("table")
    if not table or not isinstance(table, Tag):
        return results

    rows = table.find_all("tr")
    for row in rows[1:]:  # Skip header row
        cells = row.find_all("td")
        if len(cells) >= 5:
            name = cells[0].get_text(strip=True)
            ward = cells[1].get_text(strip=True)
            district = cells[2].get_text(strip=True)
            region = cells[3].get_text(strip=True)
            postcode = cells[4].get_text(strip=True)

            if name and postcode:
                results.append({
                    "name": name,
                    "ward": ward,
                    "district": district,
                    "region": region,
                    "postcode": postcode,
                })
        elif len(cells) >= 2:
            # Fallback: old 2-column format
            name = cells[0].get_text(strip=True)
            postcode = cells[-1].get_text(strip=True)
            if name and postcode:
                results.append({
                    "name": name,
                    "ward": "",
                    "district": "",
                    "region": "",
                    "postcode": postcode,
                })

    return results
