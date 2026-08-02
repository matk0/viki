FROM nousresearch/hermes-agent:v2026.7.20@sha256:f7b35053268f532f98955195c909f15a230470fbcbdacaa9fdecb95707dad04a

USER root

COPY --chmod=0755 hermes/bootstrap.sh /opt/viki-hermes/bootstrap.sh
COPY --chmod=0755 hermes/bootstrap.sh /etc/cont-init.d/03-viki-bootstrap
COPY --chmod=0644 hermes/render_config.py /opt/viki-hermes/render_config.py
COPY --chmod=0644 hermes/verify_runtime.py /opt/viki-hermes/verify_runtime.py
COPY --chmod=0644 hermes/private_gateway_proxy.py /opt/viki-hermes/private_gateway_proxy.py
COPY hermes/profiles/ /opt/viki-hermes/profiles/
COPY hermes/plugins/ /opt/viki-hermes/plugins/
COPY hermes/s6/ /etc/s6-overlay/s6-rc.d/
RUN rm -f /etc/s6-overlay/s6-rc.d/user/contents.d/dashboard && \
    rm -f /etc/cont-init.d/02-reconcile-profiles && \
    chmod -R a+rX,go-w \
    /opt/viki-hermes/profiles \
    /opt/viki-hermes/plugins \
    /etc/s6-overlay/s6-rc.d/hermes-qa \
    /etc/s6-overlay/s6-rc.d/hermes-edit \
    /etc/s6-overlay/s6-rc.d/hermes-developer \
    /etc/s6-overlay/s6-rc.d/hermes-developer-cron \
    /etc/s6-overlay/s6-rc.d/hermes-gateway-proxy && \
    chmod 0755 \
    /etc/s6-overlay/s6-rc.d/hermes-qa/run \
    /etc/s6-overlay/s6-rc.d/hermes-edit/run \
    /etc/s6-overlay/s6-rc.d/hermes-developer/run \
    /etc/s6-overlay/s6-rc.d/hermes-developer-cron/run \
    /etc/s6-overlay/s6-rc.d/hermes-gateway-proxy/run && \
    HERMES_HOME=/tmp/viki-contract \
    VIKI_HERMES_ASSETS=/opt/viki-hermes \
    HERMES_MODEL=viki-contract-model \
    /opt/viki-hermes/bootstrap.sh && \
    /command/s6-setuidgid hermes env \
    HERMES_HOME=/tmp/viki-contract \
    hermes profile info viki-qa >/dev/null && \
    /command/s6-setuidgid hermes env \
    HERMES_HOME=/tmp/viki-contract \
    hermes profile info viki-edit >/dev/null && \
    /command/s6-setuidgid hermes env \
    HERMES_HOME=/tmp/viki-contract \
    hermes profile info viki-developer >/dev/null && \
    /command/s6-setuidgid hermes env \
    HERMES_HOME=/tmp/viki-contract/profiles/viki-qa \
    HERMES_TUI_TOOLSETS=memory,clarify,viki_read \
    HERMES_TUI_PASS_SESSION_ID=0 \
    VIKI_HERMES_TOOL_TOKEN=viki-build-contract \
    python3 /opt/viki-hermes/verify_runtime.py qa && \
    /command/s6-setuidgid hermes env \
    HERMES_HOME=/tmp/viki-contract/profiles/viki-edit \
    HERMES_TUI_TOOLSETS=memory,clarify,viki_read,viki_edit \
    HERMES_TUI_PASS_SESSION_ID=0 \
    VIKI_HERMES_TOOL_TOKEN=viki-build-contract \
    python3 /opt/viki-hermes/verify_runtime.py edit && \
    /command/s6-setuidgid hermes env \
    HERMES_HOME=/tmp/viki-contract/profiles/viki-developer \
    HERMES_TUI_TOOLSETS=viki_develop \
    HERMES_TUI_PASS_SESSION_ID=0 \
    VIKI_HERMES_TOOL_TOKEN=viki-build-contract \
    python3 /opt/viki-hermes/verify_runtime.py developer && \
    rm -rf /tmp/viki-contract

CMD ["sleep", "infinity"]
