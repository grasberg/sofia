
import json
import os

def load_json(path):
    try:
        with open(path, 'r') as f:
            return json.load(f)
    except:
        return None

def load_file(path):
    try:
        with open(path, 'r') as f:
            return f.read()
    except:
        return ""

base_path = "agd_docs"

# Load RAML
raml_content = load_file(os.path.join(base_path, "arbetsgivardeklaration-hantera-redovisningsperiod-external.raml"))

# Load Schemas
schemas = {
    "grunddata": load_json(os.path.join(base_path, "schemas/grunddata.json")),
    "handelser": load_json(os.path.join(base_path, "schemas/handelser.json")),
    "kvittenser": load_json(os.path.join(base_path, "schemas/kvittenser.json")),
    "lasstatus": load_json(os.path.join(base_path, "schemas/lasstatus.json")),
    "skapagranskningsunderlagsvar": load_json(os.path.join(base_path, "schemas/skapagranskningsunderlagsvar.json")),
    "summeringsrapport": load_json(os.path.join(base_path, "schemas/summeringsrapport.json")),
    "error": load_json(os.path.join(base_path, "common_schemas/http-error-response.json"))
}

# Load Examples
examples = {
    "grunddata": load_json(os.path.join(base_path, "examples/grunddata_example.json")),
    "handelser": load_json(os.path.join(base_path, "examples/handelser_example.json")),
    "kvittenser": load_json(os.path.join(base_path, "examples/kvittenser_example.json")),
    "lasstatus_locked": load_json(os.path.join(base_path, "examples/lasstatus_locked_example.json")),
    "lasstatus_unlocked": load_json(os.path.join(base_path, "examples/lasstatus_unlocked_example.json")),
    "skapa_gransknings_false": load_json(os.path.join(base_path, "examples/skapa_granskningsunderlag_false.json")),
    "skapa_gransknings_true": load_json(os.path.join(base_path, "examples/skapa_granskningsunderlag_true.json")),
    "summeringsrapport": load_json(os.path.join(base_path, "examples/summeringsrapport_example.json")),
    "error_400": load_json(os.path.join(base_path, "common_examples/http400.json")),
    "error_404": load_json(os.path.join(base_path, "common_examples/http404.json")),
    "error_500": load_json(os.path.join(base_path, "common_examples/http500.json"))
}

doc = []
doc.append("# Arbetsgivardeklaration – hantera redovisningsperiod (v1.2.3)")
doc.append("\n## Overview")
doc.append("Detta API används för att hantera redovisningsperioder för arbetsgivardeklarationer hos Skatteverket.")
doc.append("\n**Base URI:** `https://{environment}/arbetsgivardeklaration/hanteraredovisningsperiod/v1`")
doc.append("**Protocols:** HTTPS")
doc.append("**Authentication:** OAuth 2.0 (Organisationslegitimation, System-organisationslegitimation, Personlig e-legitimation)")

doc.append("\n## Endpoints")

# Manual extraction of key info from RAML for clarity
endpoints = [
    {
        "path": "/arbetsgivare/{arbetsgivarregistrerad}/redovisningsperioder/{redovisningsperiod}/grunddata",
        "method": "GET",
        "desc": "Hämtar information från ett eget utrymme för en arbetsgivarregistrerad och redovisningsperiod. Returnerar grunddata som deklarationsdatum, anståndsdatum, tillstånd på perioden och antal inlästa uppgifter.",
        "params": [
            "arbetsgivarregistrerad: Person-/org-nummer (12 siffror)",
            "redovisningsperiod: År och månad (YYYYMM)"
        ],
        "schema": "grunddata",
        "example": "grunddata"
    },
    {
        "path": "/arbetsgivare/{arbetsgivarregistrerad}/redovisningsperioder/{redovisningsperiod}/skapaGranskningsunderlag",
        "method": "POST",
        "desc": "Verifierar och förbereder en redovisningsperiod inför inskickande. Returnerar en djuplänk för granskning/signering i Mina Sidor.",
        "params": [
            "arbetsgivarregistrerad: Person-/org-nummer (12 siffror)",
            "redovisningsperiod: År och månad (YYYYMM)",
            "lasPeriod (query): Boolean, om perioden ska låsas (default: false)"
        ],
        "schema": "skapagranskningsunderlagsvar",
        "example": "skapa_gransknings_true"
    },
    {
        "path": "/arbetsgivare/{arbetsgivarregistrerad}/redovisningsperioder/{redovisningsperiod}/handelser",
        "method": "GET",
        "desc": "Hämtar händelser loggade för redovisningsperioden i det egna utrymmet.",
        "params": [
            "arbetsgivarregistrerad: Person-/org-nummer (12 siffror)",
            "redovisningsperiod: År och månad (YYYYMM)"
        ],
        "schema": "handelser",
        "example": "handelser"
    },
    {
        "path": "/arbetsgivare/{arbetsgivarregistrerad}/redovisningsperioder/{redovisningsperiod}/lasstatus",
        "method": "GET",
        "desc": "Hämtar aktuell låsstatus för redovisningsperioden.",
        "params": [
            "arbetsgivarregistrerad: Person-/org-nummer (12 siffror)",
            "redovisningsperiod: År och månad (YYYYMM)"
        ],
        "schema": "lasstatus",
        "example": "lasstatus_locked"
    },
    {
        "path": "/arbetsgivare/{arbetsgivarregistrerad}/redovisningsperioder/{redovisningsperiod}/summeringsrapport",
        "method": "GET",
        "desc": "Hämtar en summeringsrapport för redovisningsperioden baserat på inlästa uppgifter.",
        "params": [
            "arbetsgivarregistrerad: Person-/org-nummer (12 siffror)",
            "redovisningsperiod: År och månad (YYYYMM)"
        ],
        "schema": "summeringsrapport",
        "example": "summeringsrapport"
    },
    {
        "path": "/arbetsgivare/{arbetsgivarregistrerad}/redovisningsperioder/{redovisningsperiod}/kvittenser",
        "method": "GET",
        "desc": "Hämtar kvittenser för inskickade deklarationer för perioden.",
        "params": [
            "arbetsgivarregistrerad: Person-/org-nummer (12 siffror)",
            "redovisningsperiod: År och månad (YYYYMM)"
        ],
        "schema": "kvittenser",
        "example": "kvittenser"
    }
]

for ep in endpoints:
    doc.append(f"\n### {ep['method']} {ep['path']}")
    doc.append(f"{ep['desc']}")
    doc.append("\n**Parameters:**")
    for p in ep['params']:
        doc.append(f"- {p}")
    
    doc.append("\n**Response (200 OK):**")
    doc.append("\n*Schema:*")
    doc.append("```json")
    doc.append(json.dumps(schemas.get(ep['schema']), indent=2))
    doc.append("```")
    
    doc.append("\n*Example:*")
    doc.append("```json")
    doc.append(json.dumps(examples.get(ep['example']), indent=2))
    doc.append("```")

doc.append("\n## Error Responses")
doc.append("\n### Common Errors (400, 404, 500)")
doc.append("\n**Schema:**")
doc.append("```json")
doc.append(json.dumps(schemas['error'], indent=2))
doc.append("```")

doc.append("\n**Examples:**")
doc.append("- 400 Bad Request: " + json.dumps(examples['error_400']))
doc.append("- 404 Not Found: " + json.dumps(examples['error_404']))
doc.append("- 500 Server Error: " + json.dumps(examples['error_500']))

with open("Arbetsgivardeklaration_Dokumentation.md", "w") as f:
    f.write("\n".join(doc))
