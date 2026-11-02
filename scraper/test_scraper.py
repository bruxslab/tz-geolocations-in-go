"""Tests for the Tanzania postcode scraper.

Section A: Output completeness tests — validate scraped JSON files for consistency.
Section B: Known-value spot checks — verify specific expected values.

Usage:
  python -m pytest test_scraper.py -v
"""

import json
from collections import Counter
from pathlib import Path

import pytest

DATA_DIR = Path(__file__).parent / "data"

# Deduplicated street counts per region.
EXPECTED_REGION_COUNTS = {
    "arusha": 1865,
    "dar-es-salaam": 565,
    "dodoma": 3922,
    "geita": 2579,
    "iringa": 2289,
    "kagera": 4041,
    "katavi": 1028,
    "kigoma": 1862,
    "kilimanjaro": 2659,
    "lindi": 2868,
    "manyara": 2301,
    "mara": 3122,
    "mbeya": 3400,
    "mjini-magharibi": 84,
    "morogoro": 4120,
    "mtwara": 3940,
    "mwanza": 4021,
    "njombe": 2163,
    "pemba-north": 59,
    "pemba-south": 62,
    "pwani": 1912,
    "rukwa": 2185,
    "ruvuma": 4126,
    "shinyanga": 3013,
    "simiyu": 2964,
    "singida": 2597,
    "songwe": 1715,
    "tabora": 4231,
    "tanga": 5245,
    "unguja-north": 62,
    "unguja-south": 61,
}
EXPECTED_TOTAL_STREETS = 75061


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _load(name: str) -> list[dict]:
    path = DATA_DIR / f"{name}.json"
    if not path.exists():
        pytest.skip(f"{path} not found — run the scraper first")
    return json.loads(path.read_text())


# ---------------------------------------------------------------------------
# A. Output completeness tests (against JSON files)
# ---------------------------------------------------------------------------

class TestOutputRegions:
    @pytest.fixture(scope="class")
    def regions(self):
        return _load("regions")

    def test_exactly_31_regions(self, regions):
        assert len(regions) == 31

    def test_region_fields(self, regions):
        for r in regions:
            assert r.get("name"), f"Region missing name: {r}"
            assert r.get("slug"), f"Region missing slug: {r}"

    def test_no_duplicate_slugs(self, regions):
        slugs = [r["slug"] for r in regions]
        assert len(slugs) == len(set(slugs))

    def test_all_expected_regions_present(self, regions):
        slugs = {r["slug"] for r in regions}
        for expected_slug in EXPECTED_REGION_COUNTS:
            assert expected_slug in slugs, f"Missing region: {expected_slug}"


class TestOutputDistricts:
    @pytest.fixture(scope="class")
    def districts(self):
        return _load("districts")

    @pytest.fixture(scope="class")
    def regions(self):
        return _load("regions")

    def test_every_region_has_districts(self, districts, regions):
        region_slugs_with_districts = {d["region_slug"] for d in districts}
        for r in regions:
            assert r["slug"] in region_slugs_with_districts, (
                f"Region '{r['name']}' has no districts"
            )

    def test_no_duplicate_district_pairs(self, districts):
        pairs = [(d["region_slug"], d["slug"]) for d in districts]
        assert len(pairs) == len(set(pairs)), "Duplicate (region, district) pairs"

    def test_district_fields(self, districts):
        for d in districts:
            assert d.get("name")
            assert d.get("slug")
            assert d.get("region_slug")

    def test_referential_integrity_to_regions(self, districts, regions):
        valid_region_slugs = {r["slug"] for r in regions}
        for d in districts:
            assert d["region_slug"] in valid_region_slugs, (
                f"District '{d['name']}' references unknown region '{d['region_slug']}'"
            )


class TestOutputWards:
    @pytest.fixture(scope="class")
    def wards(self):
        return _load("wards")

    @pytest.fixture(scope="class")
    def districts(self):
        return _load("districts")

    def test_every_district_has_wards(self, wards, districts):
        district_keys_with_wards = {
            (w["region_slug"], w["district_slug"]) for w in wards
        }
        for d in districts:
            key = (d["region_slug"], d["slug"])
            assert key in district_keys_with_wards, (
                f"District '{d['name']}' ({d['region_slug']}) has no wards"
            )

    def test_no_duplicate_ward_tuples(self, wards):
        tuples = [(w["region_slug"], w["district_slug"], w["slug"]) for w in wards]
        assert len(tuples) == len(set(tuples)), "Duplicate (region, district, ward) tuples"

    def test_all_wards_have_postcode(self, wards):
        missing = [w["name"] for w in wards if not w.get("postcode")]
        assert not missing, (
            f"{len(missing)} wards missing postcode, first 10: {missing[:10]}"
        )

    def test_ward_postcodes_are_valid(self, wards):
        for w in wards:
            pc = w.get("postcode", "")
            if pc:
                assert pc.isdigit() and len(pc) == 5, (
                    f"Ward '{w['name']}' has invalid postcode: '{pc}'"
                )

    def test_ward_fields(self, wards):
        for w in wards:
            assert w.get("name")
            assert w.get("slug")
            assert w.get("district_slug")
            assert w.get("region_slug")

    def test_referential_integrity_to_districts(self, wards, districts):
        valid_keys = {(d["region_slug"], d["slug"]) for d in districts}
        for w in wards:
            key = (w["region_slug"], w["district_slug"])
            assert key in valid_keys, (
                f"Ward '{w['name']}' references unknown district "
                f"'{w['district_slug']}' in region '{w['region_slug']}'"
            )


class TestOutputStreets:
    @pytest.fixture(scope="class")
    def streets(self):
        return _load("streets")

    @pytest.fixture(scope="class")
    def wards(self):
        return _load("wards")

    def test_total_streets(self, streets):
        assert len(streets) == EXPECTED_TOTAL_STREETS, (
            f"Expected {EXPECTED_TOTAL_STREETS} streets, got {len(streets)}"
        )

    def test_streets_per_region(self, streets):
        counts = Counter(s["region_slug"] for s in streets)
        for slug, expected in EXPECTED_REGION_COUNTS.items():
            actual = counts.get(slug, 0)
            assert actual == expected, (
                f"Region '{slug}': expected {expected} streets, got {actual}"
            )

    def test_every_ward_has_streets(self, streets, wards):
        ward_keys_with_streets = {
            (s["region_slug"], s["district_slug"], s["ward_slug"]) for s in streets
        }
        missing = []
        for w in wards:
            key = (w["region_slug"], w["district_slug"], w["slug"])
            if key not in ward_keys_with_streets:
                missing.append(f"{w['name']} ({w['district_slug']}/{w['region_slug']})")
        assert not missing, (
            f"{len(missing)} wards have no streets, first 10: {missing[:10]}"
        )

    def test_all_streets_have_postcode(self, streets):
        missing = [s["name"] for s in streets if not s.get("postcode")]
        assert not missing, (
            f"{len(missing)} streets missing postcode, first 10: {missing[:10]}"
        )

    def test_street_postcodes_are_valid(self, streets):
        bad = []
        for s in streets:
            pc = s.get("postcode", "")
            if not pc.isdigit() or len(pc) != 5:
                bad.append(f"{s['name']}: '{pc}'")
        assert not bad, f"{len(bad)} invalid postcodes, first 10: {bad[:10]}"

    def test_street_fields(self, streets):
        for s in streets:
            assert s.get("name"), f"Street missing name: {s}"
            assert s.get("postcode"), f"Street missing postcode: {s}"
            assert s.get("ward_slug"), f"Street missing ward_slug: {s}"
            assert s.get("district_slug"), f"Street missing district_slug: {s}"
            assert s.get("region_slug"), f"Street missing region_slug: {s}"

    def test_referential_integrity_to_wards(self, streets, wards):
        valid_keys = {
            (w["region_slug"], w["district_slug"], w["slug"]) for w in wards
        }
        orphans = []
        for s in streets:
            key = (s["region_slug"], s["district_slug"], s["ward_slug"])
            if key not in valid_keys:
                orphans.append(
                    f"{s['name']} -> {s['ward_slug']}/{s['district_slug']}/{s['region_slug']}"
                )
        assert not orphans, (
            f"{len(orphans)} streets reference unknown wards, first 10: {orphans[:10]}"
        )

    def test_no_duplicate_streets_per_ward(self, streets):
        tuples = [
            (s["region_slug"], s["district_slug"], s["ward_slug"], s["name"])
            for s in streets
        ]
        dupes = len(tuples) - len(set(tuples))
        assert dupes == 0, f"{dupes} duplicate street entries"


# ---------------------------------------------------------------------------
# B. Known-value spot checks
# ---------------------------------------------------------------------------

class TestSpotChecks:
    @pytest.fixture(scope="class")
    def districts(self):
        return _load("districts")

    @pytest.fixture(scope="class")
    def wards(self):
        return _load("wards")

    @pytest.fixture(scope="class")
    def streets(self):
        return _load("streets")

    def test_arusha_has_7_districts(self, districts):
        arusha_districts = [d for d in districts if d["region_slug"] == "arusha"]
        assert len(arusha_districts) == 7, (
            f"Expected 7 Arusha districts, got {len(arusha_districts)}: "
            f"{[d['name'] for d in arusha_districts]}"
        )

    def test_arumeru_has_26_wards(self, wards):
        arumeru_wards = [
            w for w in wards
            if w["district_slug"] == "arumeru" and w["region_slug"] == "arusha"
        ]
        assert len(arumeru_wards) == 26, (
            f"Expected 26 Arumeru wards, got {len(arumeru_wards)}"
        )

    def test_akheri_has_13_streets(self, streets):
        akheri_streets = [
            s for s in streets
            if s["ward_slug"] == "akheri"
            and s["district_slug"] == "arumeru"
            and s["region_slug"] == "arusha"
        ]
        assert len(akheri_streets) == 13, (
            f"Expected 13 Akheri streets, got {len(akheri_streets)}"
        )

    def test_akheri_postcode_23306(self, streets, wards):
        akheri_streets = [
            s for s in streets
            if s["ward_slug"] == "akheri"
            and s["district_slug"] == "arumeru"
        ]
        for s in akheri_streets:
            assert s["postcode"] == "23306"

        akheri_ward = [
            w for w in wards
            if w["slug"] == "akheri"
            and w["district_slug"] == "arumeru"
        ]
        assert len(akheri_ward) == 1
        assert akheri_ward[0]["postcode"] == "23306"

    def test_dar_es_salaam_streets(self, streets):
        dar_streets = [s for s in streets if s["region_slug"] == "dar-es-salaam"]
        assert len(dar_streets) == 565, (
            f"Expected 565 Dar es Salaam streets, got {len(dar_streets)}"
        )

    def test_tanga_streets(self, streets):
        tanga_streets = [s for s in streets if s["region_slug"] == "tanga"]
        assert len(tanga_streets) == 5245, (
            f"Expected 5245 Tanga streets, got {len(tanga_streets)}"
        )

    def test_smallest_regions(self, streets):
        """Unguja South should have exactly 61 streets (smallest region)."""
        us_streets = [s for s in streets if s["region_slug"] == "unguja-south"]
        assert len(us_streets) == 61
