"""Validate OpenAPI 3.1 and recorded source/Go responses with its JSON Schemas."""
import json
import re
import sys
from pathlib import Path
from urllib.parse import urlsplit

from jsonschema import Draft202012Validator
from openapi_spec_validator import validate

root = Path(__file__).resolve().parents[1]
spec = json.loads((root / "docs/openapi.yaml").read_text())
validate(spec)
for name, schema in spec["components"]["schemas"].items():
    Draft202012Validator.check_schema(schema)

paths = [root / "tests/contract/testdata/reference.json"]
paths.extend(Path(p) for p in sys.argv[1:])
count = 0
for path in paths:
    for case in json.loads(path.read_text())["cases"]:
        actual = urlsplit(case["path"]).path
        route = next(
            (v for k, v in spec["paths"].items()
             if re.fullmatch(re.sub(r"\{[^}]+\}", "[^/]+", k), actual)),
            None,
        )
        schema = {"$ref": "#/components/schemas/Error"}
        if case["status"] == 200:
            if route is None:
                raise AssertionError(f"Undocumented route: {actual}")
            schema = route["get"]["responses"]["200"]["content"]["application/json"]["schema"]
        combined = {**schema, "components": spec["components"]}
        Draft202012Validator(combined).validate(case["body"])
        count += 1
print(f"OpenAPI 3.1 valid; {count} response fixtures match documented schemas")
