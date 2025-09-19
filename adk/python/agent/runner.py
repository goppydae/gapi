import sys
import json
import runpy

REQUIRED = ["ID", "NAME", "VERSION", "TYPE"]
HOOKS = ["initialize", "start", "stop", "restart", "reload"]

def main():
    if len(sys.argv) != 3 or sys.argv[2] != "--describe":
        print("Usage: runner.py <agentfile> --describe", file=sys.stderr)
        sys.exit(1)

    agent_path = sys.argv[1]
    g = runpy.run_path(agent_path)

    missing = [k for k in REQUIRED if k not in g]
    if missing:
        print(f"Missing fields: {', '.join(missing)}", file=sys.stderr)
        sys.exit(2)

    out = {
        "id": g["ID"],
        "name": g["NAME"],
        "version": g["VERSION"],
        "type": g["TYPE"],
        "description": g.get("DESCRIPTION", ""),
        "interval": g.get("INTERVAL", 0),
        "enabled": g.get("ENABLED", True),
        "implements": [fn for fn in HOOKS if fn in g],
    }

    print(json.dumps(out))

if __name__ == "__main__":
    main()
