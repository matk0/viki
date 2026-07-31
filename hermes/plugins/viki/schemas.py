_STEP = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "keyword": {
            "type": "string",
            "enum": ["given", "when", "then", "and", "but"],
        },
        "text": {"type": "string"},
    },
    "required": ["keyword", "text"],
}

_REFERENCE = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "targetPageId": {"type": ["string", "null"]},
        "targetClientKey": {"type": "string"},
        "targetTitle": {
            "type": "string",
            "description": "Kanonický názov cieľového pojmu z viki alebo z rovnakej sady zmien.",
        },
        "relation": {"type": "string"},
    },
    "required": ["targetPageId", "targetClientKey", "targetTitle", "relation"],
}

_CONTENT = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "title": {"type": "string"},
        "bodyMd": {"type": "string"},
        "aliases": {"type": "array", "items": {"type": "string"}},
        "steps": {"type": "array", "items": _STEP},
        "references": {"type": "array", "items": _REFERENCE},
    },
    "required": [
        "title",
        "bodyMd",
        "aliases",
        "steps",
        "references",
    ],
}

_OPERATION = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "operation": {"type": "string", "enum": ["create", "revise"]},
        "clientKey": {"type": "string"},
        "pageId": {"type": ["string", "null"]},
        "baseRevisionId": {"type": ["string", "null"]},
        "kind": {
            "type": "string",
            "enum": ["primitive", "scenario", "subscenario"],
        },
        "primitiveKind": {
            "type": ["string", "null"],
            "enum": ["noun", "verb", None],
        },
        "parentId": {"type": ["string", "null"]},
        "parentClientKey": {"type": "string"},
        "slug": {"type": "string"},
        "content": _CONTENT,
    },
    "required": [
        "operation",
        "clientKey",
        "pageId",
        "baseRevisionId",
        "kind",
        "primitiveKind",
        "parentId",
        "parentClientKey",
        "slug",
        "content",
    ],
}

SEARCH = {
    "name": "search_viki",
    "description": (
        "Vyhľadaj relevantné stránky viki. Použi pred odpoveďou na otázku "
        "aj pred návrhom úprav. Vyhľadávanie vždy zahŕňa schválené aj aktuálne konceptové revízie."
    ),
    "parameters": {
        "type": "object",
        "additionalProperties": False,
        "properties": {
            "query": {"type": "string", "description": "Slovenský vyhľadávací dopyt."},
            "limit": {"type": "integer", "minimum": 1, "maximum": 20},
        },
        "required": ["query"],
    },
}

GET_PAGE = {
    "name": "get_viki_page",
    "description": "Načítaj stránku viki a jej aktuálne revízie podľa ID stránky.",
    "parameters": {
        "type": "object",
        "additionalProperties": False,
        "properties": {"pageId": {"type": "string"}},
        "required": ["pageId"],
    },
}

GET_REVISION = {
    "name": "get_viki_revision",
    "description": (
        "Načítaj presnú nemennú revíziu podľa ID. Použi ju na citáciu alebo ako "
        "baseRevisionId pri úprave."
    ),
    "parameters": {
        "type": "object",
        "additionalProperties": False,
        "properties": {"revisionId": {"type": "string"}},
        "required": ["revisionId"],
    },
}

PROPOSE_CHANGESET = {
    "name": "propose_viki_changeset",
    "description": (
        "Priprav návrh nových stránok alebo revízií na kontrolu človekom. Nástroj nič "
        "nevytvorí ani nepublikuje. Použi ho po získaní presných ID a vyjasnení nejednoznačností. "
        "Scenár musí mať v references všetky použité pojmy. Chýbajúci pojem pridaj "
        "ako skoršiu primitive operáciu a prepoj ho cez targetClientKey. Pre kind=scenario "
        "steps musí byť prázdne; BDD kroky patria iba do kind=subscenario."
    ),
    "parameters": {
        "type": "object",
        "additionalProperties": False,
        "properties": {
            "summary": {"type": "string"},
            "operations": {"type": "array", "minItems": 1, "items": _OPERATION},
        },
        "required": ["summary", "operations"],
    },
}

ALL = [SEARCH, GET_PAGE, GET_REVISION, PROPOSE_CHANGESET]
