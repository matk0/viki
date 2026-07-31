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
        name="propose_viki_changeset",
        toolset="viki_edit",
        schema=schemas.PROPOSE_CHANGESET,
        handler=tools.propose_viki_changeset,
    )
