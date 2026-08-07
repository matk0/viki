from . import history_projection, schemas, tools


def register(ctx):
    history_projection.install()
    ctx.register_tool(
        name="search_viki",
        toolset="viki_read",
        schema=schemas.SEARCH,
        handler=tools.search_viki,
    )
    ctx.register_tool(
        name="get_viki_page",
        toolset="viki_read",
        schema=schemas.GET_PAGE,
        handler=tools.get_viki_page,
    )
    ctx.register_tool(
        name="get_viki_revision",
        toolset="viki_read",
        schema=schemas.GET_REVISION,
        handler=tools.get_viki_revision,
    )
    ctx.register_tool(
        name="apply_viki_draft_changeset",
        toolset="viki_edit",
        schema=schemas.APPLY_DRAFT_CHANGESET,
        handler=tools.apply_viki_draft_changeset,
    )
    ctx.register_tool(
        name="claim_next_scenario",
        toolset="viki_develop",
        schema=schemas.CLAIM_NEXT_SCENARIO,
        handler=tools.claim_next_scenario,
    )
    ctx.register_tool(
        name="complete_scenario_development",
        toolset="viki_develop",
        schema=schemas.COMPLETE_SCENARIO_DEVELOPMENT,
        handler=tools.complete_scenario_development,
    )
    ctx.register_tool(
        name="block_scenario_development",
        toolset="viki_develop",
        schema=schemas.BLOCK_SCENARIO_DEVELOPMENT,
        handler=tools.block_scenario_development,
    )
