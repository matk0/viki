_STEP = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "keyword": {
            "type": "string",
            "enum": ["given", "when", "then", "and", "but"],
        },
        "definitionId": {"type": ["string", "null"]},
        "expression": {"type": "string"},
        "arguments": {"type": "array", "items": {"type": "string"}},
        "text": {"type": "string"},
    },
    "required": ["keyword", "definitionId", "expression", "arguments", "text"],
}

_REFERENCE = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "targetPageId": {"type": ["string", "null"]},
        "targetClientKey": {"type": "string"},
        "targetTitle": {
            "type": "string",
            "description": "Kanonický názov cieľového konceptu z viki alebo z rovnakej sady zmien.",
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
        "steps": {"type": "array", "items": _STEP},
        "references": {"type": "array", "items": _REFERENCE},
    },
    "required": [
        "title",
        "bodyMd",
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
            "enum": ["concept", "feature", "scenario"],
        },
        "conceptKind": {
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
        "conceptKind",
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
        "aj pred návrhom úprav. Vyhľadávanie vždy zahŕňa schválené aj aktuálne draftové revízie."
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

APPLY_DRAFT_CHANGESET = {
    "name": "apply_viki_draft_changeset",
    "description": (
        "Vytvor atómovo nové draftové stránky alebo draftové revízie na kontrolu človekom. "
        "Nástroj drafty vytvorí, ale nikdy ich neschváli ani nepublikuje. Použi ho po získaní "
        "presných ID a vyjasnení nejednoznačností. "
        "Funkcia aj scenár musia mať v references všetky použité koncepty. Chýbajúci koncept pridaj "
        "ako skoršiu concept operáciu a prepoj ho cez targetClientKey. Pre kind=feature "
        "steps musí byť prázdne; BDD kroky patria iba do kind=scenario, ktorý má rodičovskú feature. "
        "Pri krokoch scenára opätovne použi definitionId zo stepDefinitions v search_viki vždy, keď existuje "
        "významovo zodpovedajúca definícia. Novú expression navrhni iba vtedy, keď zodpovedajúca definícia neexistuje. "
        "Každá nová funkcia musí mať v tej istej sade zmien aspoň jeden scenár vytvorený po nej "
        "a prepojený na funkciu cez parentClientKey."
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

CLAIM_NEXT_SCENARIO = {
    "name": "claim_next_scenario",
    "description": "Vyzdvihni najstarší schválený scenár, ktorý čaká na vývoj.",
    "parameters": {
        "type": "object",
        "additionalProperties": False,
        "properties": {},
    },
}

COMPLETE_SCENARIO_DEVELOPMENT = {
    "name": "complete_scenario_development",
    "description": "Odošli implementáciu aktuálneho scenára do cieľového systému a označ ho ako vyvinutý.",
    "parameters": {
        "type": "object",
        "additionalProperties": False,
        "properties": {
            "implementation": {"type": "string", "minLength": 1},
        },
        "required": ["implementation"],
    },
}

BLOCK_SCENARIO_DEVELOPMENT = {
    "name": "block_scenario_development",
    "description": "Označ aktuálny scenár ako zablokovaný, ak ho nemožno implementovať.",
    "parameters": {
        "type": "object",
        "additionalProperties": False,
        "properties": {
            "reason": {"type": "string", "minLength": 1},
        },
        "required": ["reason"],
    },
}

ALL = [
    SEARCH,
    GET_PAGE,
    GET_REVISION,
    APPLY_DRAFT_CHANGESET,
    CLAIM_NEXT_SCENARIO,
    COMPLETE_SCENARIO_DEVELOPMENT,
    BLOCK_SCENARIO_DEVELOPMENT,
]
