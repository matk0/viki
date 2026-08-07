#!/bin/sh
set -eu

HERMES_HOME="${HERMES_HOME:-/opt/data}"
VIKI_HERMES_ASSETS="${VIKI_HERMES_ASSETS:-/opt/viki-hermes}"
VIKI_HERMES_CLI="${VIKI_HERMES_CLI:-hermes}"

if [ "$(id -u)" = 0 ] && [ "${VIKI_BOOTSTRAP_AS_HERMES:-}" != 1 ]; then
    exec /command/s6-setuidgid hermes env \
        HERMES_HOME="$HERMES_HOME" \
        VIKI_HERMES_ASSETS="$VIKI_HERMES_ASSETS" \
        VIKI_HERMES_CLI="$VIKI_HERMES_CLI" \
        VIKI_BOOTSTRAP_AS_HERMES=1 \
        HERMES_MODEL="${HERMES_MODEL:-gpt-5.6-terra}" \
        OPENAI_BASE_URL="${OPENAI_BASE_URL:-https://api.openai.com/v1}" \
        "$0"
fi

fail_if_symlink() {
    if [ -L "$1" ]; then
        echo "[viki-hermes] refusing managed symlink: $1" >&2
        exit 1
    fi
}

install_managed_file() {
    source_file="$1"
    target_file="$2"
    mode="$3"
    fail_if_symlink "$target_file"
    temporary_file="${target_file}.tmp.$$"
    cp "$source_file" "$temporary_file"
    chmod "$mode" "$temporary_file"
    mv -f "$temporary_file" "$target_file"
}

prepare_distribution() {
    profile_name="$1"
    template_name="$2"
    distribution_root="$HERMES_HOME/.viki-distributions"
    distribution_dir="$distribution_root/$profile_name"

    fail_if_symlink "$distribution_root"
    fail_if_symlink "$distribution_dir"
    fail_if_symlink "$distribution_dir/plugins"
    fail_if_symlink "$distribution_dir/plugins/viki"
    fail_if_symlink "$distribution_dir/scripts"
    mkdir -p "$distribution_dir/plugins/viki" "$distribution_dir/scripts"

    python3 "$VIKI_HERMES_ASSETS/render_config.py" \
        "$VIKI_HERMES_ASSETS/profiles/$template_name/config.yaml" \
        "$distribution_dir/config.yaml"

    for profile_file in distribution.yaml SOUL.md .no-bundled-skills; do
        install_managed_file \
            "$VIKI_HERMES_ASSETS/profiles/$template_name/$profile_file" \
            "$distribution_dir/$profile_file" \
            0640
    done

    for plugin_file in plugin.yaml __init__.py history_projection.py schemas.py tools.py; do
        install_managed_file \
            "$VIKI_HERMES_ASSETS/plugins/viki/$plugin_file" \
            "$distribution_dir/plugins/viki/$plugin_file" \
            0640
    done

	if [ -f "$VIKI_HERMES_ASSETS/profiles/$template_name/check_queue.py" ]; then
		install_managed_file \
			"$VIKI_HERMES_ASSETS/profiles/$template_name/check_queue.py" \
			"$distribution_dir/scripts/check_queue.py" \
			0750
	fi

    printf '%s\n' "$distribution_dir"
}

bootstrap_profile() {
    profile_name="$1"
    template_name="$2"
    profile_home="$HERMES_HOME/profiles/$profile_name"
    distribution_dir="$(prepare_distribution "$profile_name" "$template_name")"

    fail_if_symlink "$HERMES_HOME/profiles"
    fail_if_symlink "$profile_home"
    fail_if_symlink "$profile_home/distribution.yaml"

    if [ -f "$profile_home/distribution.yaml" ]; then
        "$VIKI_HERMES_CLI" profile update "$profile_name" --force-config --yes
    else
        "$VIKI_HERMES_CLI" profile install "$distribution_dir" \
            --name "$profile_name" \
            --force \
            --yes
    fi
}

fail_if_symlink "$HERMES_HOME"
mkdir -p "$HERMES_HOME/profiles" "$HERMES_HOME/.viki-distributions"
bootstrap_profile viki-qa qa
bootstrap_profile viki-edit edit
bootstrap_profile viki-developer developer
