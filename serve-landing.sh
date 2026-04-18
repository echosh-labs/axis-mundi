#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
LANDING_DIR="$ROOT_DIR/landing"
DIST_DIR="$LANDING_DIR/dist"
HTML_FILE="$LANDING_DIR/index.html"
PORT="8888"
NOBUILD=0
BUILDONLY=0

# Simple CLI
while [[ $# -gt 0 ]]; do
    case "$1" in
        -p|--port)
            PORT="$2"; shift 2;;
        --no-build)
            NOBUILD=1; shift;;
        --build-only)
            BUILDONLY=1; shift;;
        -h|--help)
            cat <<EOF
Usage: $(basename "$0") [-p PORT] [--no-build] [--build-only]
    -p, --port     Port to serve (default: 8888)
    --no-build     Skip npm install/build steps and just serve existing files
    --build-only   Only install/build and update bindings, do not start server
EOF
            exit 0;;
        *) echo "Unknown arg: $1"; exit 2;;
    esac
done

if [[ ! -d "$LANDING_DIR" ]]; then
    echo "Landing directory not found at $LANDING_DIR" >&2
    exit 1
fi

cd "$LANDING_DIR"

if [[ $NOBUILD -eq 0 ]]; then
    if [[ ! -d node_modules ]]; then
        echo "Installing landing dependencies..."
        npm install
    fi

    echo "Building Tailwind bundle..."
    if ! npm run build >/dev/null; then
        echo "npm build failed; ensure Node and Tailwind are installed" >&2
        exit 1
    fi

    CSS_FILE="$DIST_DIR/landing.css"
    if [[ ! -f "$CSS_FILE" ]]; then
        echo "Expected CSS at $CSS_FILE after build" >&2
        exit 1
    fi

    # compute SHA384 base64; prefer openssl, fallback to sha384sum
    if command -v openssl >/dev/null 2>&1; then
        HASH=$(openssl dgst -sha384 -binary "$CSS_FILE" | base64)
    else
        HASH=$(sha384sum "$CSS_FILE" | awk '{print $1}' | xxd -r -p | base64)
    fi
    SHORT_HASH=$(echo "${HASH:0:8}" | tr '+/' '-_')
    HASHED_CSS="$DIST_DIR/landing.${SHORT_HASH}.css"

    # move into place (force overwrite if exists)
    mv -f "$CSS_FILE" "$HASHED_CSS"

    python3 - "$HTML_FILE" "$SHORT_HASH" "$HASH" <<'PY'
import sys, re, pathlib
html_paths = [pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[1]).parent / 'mcp' / 'index.html']
short_hash = sys.argv[2]
full_hash = sys.argv[3]
pattern = r'href="([^"]*\/)landing\.[a-zA-Z0-9_\-]+\.css" (x?)integrity="[^"]+"'

for html_path in html_paths:
    if not html_path.exists(): continue
    html = html_path.read_text()
    
    def replacer(m):
        prefix = m.group(1)
        x_int = m.group(2)
        new_href = f"{prefix}landing.{short_hash}.css"
        new_integrity = f"sha384-{full_hash}"
        return f'href="{new_href}" {x_int}integrity="{new_integrity}"'
        
    updated = re.sub(pattern, replacer, html)
    if html == updated:
        print(f"Warning: link tag not updated in {html_path.name}; pattern not found", file=sys.stderr)
    html_path.write_text(updated)
PY
fi

if [[ $BUILDONLY -eq 1 ]]; then
    echo "Build complete. Exiting (--build-only)."
    exit 0
fi

echo "Serving landing/ at http://localhost:${PORT} (Ctrl+C to stop...)"
# bind to localhost explicitly for safety
if command -v python3 >/dev/null 2>&1; then
    # Ensure bucky-ball/vertices.json exists for the viewer; try to generate if missing
    BALL_DIR="$LANDING_DIR/bucky-ball"
    JSON_FILE="$BALL_DIR/vertices.json"
    if [[ ! -f "$JSON_FILE" ]]; then
        if command -v go >/dev/null 2>&1; then
            echo "Generating $JSON_FILE from Go program..."
            (cd "$BALL_DIR" && go run main.go -json vertices.json) || echo "Warning: failed to generate vertices.json"
        else
            echo "go tool not found; viewer may fail without $JSON_FILE"
        fi
    fi

    python3 -m http.server "$PORT" --bind 127.0.0.1 --directory "$LANDING_DIR"
else
    echo "python3 not found; cannot start simple HTTP server" >&2
    exit 1
fi
