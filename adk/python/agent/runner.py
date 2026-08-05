#! /usr/bin/env python3
# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

import argparse, importlib.util, json, os, sys, signal, threading, time, inspect, traceback, socket
from importlib.machinery import SourceFileLoader
from contextlib import redirect_stdout, redirect_stderr

# Ensure we can import the 'gapi' package from the parent directory
sys.path.append(os.path.join(os.path.dirname(__file__), ".."))


# gopy's generated gapi/native/adk.py chdirs into its own directory so
# dlopen can find _adk.so, and restores the caller's directory only on
# the success path. When the extension was never built the ImportError
# escapes with the process parked in adk/python/gapi/native, and every
# relative path handed to this runner afterwards resolves against the
# wrong root (GAPI-DIV-025). The runner owns its working directory, so
# it restores it here rather than trusting generated code to unwind.
_entry_cwd = os.getcwd()
try:
    if os.getenv("ADK_FORCE_DUMMY"):
        raise ImportError("Forced Dummy ADK")
    from gapi.native import adk
except ImportError as e:
    if os.getenv("ADK_REJECT_DUMMY"):
        print(f"[FATAL] Failed to import gapi.native.adk and ADK_REJECT_DUMMY is set: {e}", file=sys.stderr)
        sys.exit(1)
    
    print(f"DEBUG: Failed to import gapi.native.adk: {e}", file=sys.stderr)
    # Use a dummy adk if bindings fail (for bootstrapping/testing without compilation)
    class DummyAdk:
        def Initialize(self, n, v, t): pass
        def SendEvent(self, msg): 
            try:
                # Use __stdout__ to bypass any redirection (e.g. during agent start)
                sys.__stdout__.write(msg + "\n")
                sys.__stdout__.flush()
            except BrokenPipeError:
                pass
        def StartHeartbeat(self, id, t): pass
        def SetSchemaHash(self, h): pass
        def ComputeSchemaHash(self, f): return "dummy_hash"
        def StartQUIC(self, addr): pass
    
    # Instantiate the dummy ADK
    adk = DummyAdk()
    
    # Warn if not explicitly forced
    if not os.getenv("ADK_FORCE_DUMMY"):
        print("[WARNING] Running with DummyAdk (stdout fallback mode). "
              "This should only be used for development/testing. "
              "Set ADK_FORCE_DUMMY=1 to suppress this warning.",
              file=sys.stderr)
finally:
    # Unconditional: this block never leaves the process somewhere the
    # caller did not put it, whatever the generated importer did.
    os.chdir(_entry_cwd)

try:
    from gapi.schemas import AgentMetadata
except ImportError:
    # Fallback to simple dict if gapi package is missing (should verify this doesn't happen in production)
    AgentMetadata = dict 

FN_ALIASES = {
    "init":    ["initialize", "init", "setup"],
    "start":   ["start", "run"],
    "stop":    ["stop", "shutdown", "teardown"],
    "reload":  ["reload", "rehash", "reconfigure"],
    "restart": ["restart"],
}

META_KEYS = {
    "id": ["ID","id","agent_id"], "name":["NAME","name"], "version":["VERSION","version"],
    "type":["TYPE","type","unit_type","kind"], "enabled":["ENABLED","enabled"],
    "interval":["INTERVAL","interval"],
    "requires":["REQUIRES","requires","DEPS","deps","Dependencies"],
    "wants":["WANTS","wants"],
    "wanted_by":["WANTED_BY","wanted_by","WantedBy"],
    "required_by":["REQUIRED_BY","required_by","RequiredBy"],
    "listen_stream": ["LISTEN_STREAM", "SOCKET", "PORT"],
    "cpu_limit": ["CPU_LIMIT", "CPU"],
    "memory_limit": ["MEMORY_LIMIT", "MEM", "MEMORY"],
    "schedule": ["SCHEDULE"],
    "desc":["DESCRIPTION","description","Desc"],
}

# Schema version for agent metadata
SCHEMA_VERSION = "1.0.0"

READY_TIMEOUT_SEC = float(os.getenv("RUNNER_READY_TIMEOUT", "20"))
GRACE_SEC = float(os.getenv("RUNNER_READY_GRACE", "0.25"))
READY_MODE = os.getenv("RUNNER_READY_MODE", "strict").lower()  # strict by default now
RUN_ID = os.getenv("ADK_RUN_ID", "")

def _notify(event: str, **kv):
    kv.setdefault("ts", time.time())
    if RUN_ID:
        kv.setdefault("run_id", RUN_ID)
    msg = json.dumps({"event": event, **kv}, ensure_ascii=False)
    adk.SendEvent(msg)


def _get_fn(mod, keys):
    for k in keys:
        f = getattr(mod, k, None)
        if callable(f):
            return f
    return None

def load_module(path):
    path = os.path.abspath(path)
    with redirect_stdout(sys.stderr), redirect_stderr(sys.stderr):
        loader = SourceFileLoader("agent_mod", path)
        spec = importlib.util.spec_from_loader("agent_mod", loader)
        if spec is None:
            raise ImportError(f"no loader available for path: {path}")
        mod = importlib.util.module_from_spec(spec)
        loader.exec_module(mod)
        return mod

def _ports_from_meta(mod):
    ports = []
    p = getattr(mod, "PORTS", None)
    if isinstance(p, (list, tuple)):
        for x in p:
            try: ports.append(int(x))
            except Exception: pass
    b = getattr(mod, "BIND", None)
    if isinstance(b, (list, tuple)) and len(b) == 2:
        try: ports.append(int(b[1]))
        except Exception: pass
    return list(dict.fromkeys(ports))

def _is_port_open(port, host="127.0.0.1", timeout=0.15):
    try:
        with socket.create_connection((host, port), timeout=timeout):
            return True
    except Exception:
        return False

class AgentWrapper:
    def __init__(self, mod, agent_id: str, agent_type: str):
        self.mod = mod
        self.agent_id = agent_id
        self.agent_type = (agent_type or "service").lower()
        # A one-shot agent runs start() to completion and exits. A timer is
        # invoked once per fire by TimerAgent.execute, which waits for the
        # process; treating it as a service made every fire block on the
        # readiness poll and then on the supervision loop, so the interval
        # was never honoured (GAPI-DIV-039).
        self.oneshot = self.agent_type in ("timer", "oneshot")
        self.state = "inactive"
        self.stop_evt = threading.Event()
        self._start_thread = None

        self.fn_init   = _get_fn(mod, FN_ALIASES["init"])
        self.fn_start  = _get_fn(mod, FN_ALIASES["start"])
        self.fn_stop   = _get_fn(mod, FN_ALIASES["stop"])
        self.fn_reload = _get_fn(mod, FN_ALIASES["reload"])
        self.fn_restart= _get_fn(mod, FN_ALIASES["restart"])

        self._ports = _ports_from_meta(mod)

    def _call_start(self):
        if not self.fn_start:
            return
        sig = inspect.signature(self.fn_start)
        with redirect_stdout(sys.stderr), redirect_stderr(sys.stderr):
            if len(sig.parameters) == 0:
                self.fn_start()
            else:
                self.fn_start(self.stop_evt)


    def _await_ready(self):
        deadline = time.time() + READY_TIMEOUT_SEC
        baseline = {t.ident for t in threading.enumerate() if not t.daemon}
        time.sleep(GRACE_SEC)

        def new_non_daemon_threads():
            current = [t for t in threading.enumerate() if not t.daemon]
            return [t for t in current if t.ident not in baseline]

        # If no start function, treat as ready
        if self.fn_start is None:
            return True

        while time.time() < deadline:
            # a) start thread alive (work running)
            if self._start_thread and self._start_thread.is_alive():
                return True
            # b) start thread already returned, but new worker threads exist
            if new_non_daemon_threads():
                return True
            # c) declared port opened
            for p in self._ports:
                if _is_port_open(p):
                    return True
            # d) loose mode fallback (opt-in via env)
            if READY_MODE == "loose":
                return True

            _notify("start_pending", id=self.agent_id, type=self.agent_type)
            if self.stop_evt.wait(0.2):
                break
        return False

    def initialize(self):
        if self.stop_evt.is_set():
            self.stop_evt = threading.Event()
        if self.fn_init:
            with redirect_stdout(sys.stderr), redirect_stderr(sys.stderr):
                self.fn_init()
        self.state = "initialized"
        _notify("initialized", id=self.agent_id, type=self.agent_type)

    def start(self):
        if self.stop_evt.is_set():
            self.stop_evt = threading.Event()

        self.state = "starting"
        _notify("starting", id=self.agent_id, type=self.agent_type)

        if self.oneshot:
            # Synchronous, and no readiness poll: for a one-shot, "ready"
            # is indistinguishable from "finished". _await_ready looks for
            # a live start thread, new worker threads or an open port, and
            # a timer body has none of those, so it would spin to its full
            # 20-second deadline and then report not-ready.
            try:
                self._call_start()
            except Exception as e:
                self.state = "failed"
                _notify("error", error=str(e), id=self.agent_id, type=self.agent_type)
                raise
            self.state = "completed"
            _notify("completed", state=self.state, id=self.agent_id, type=self.agent_type)
            return

        err_holder = {"err": None}
        def runner():
            try:
                self._call_start()
            except Exception as e:
                err_holder["err"] = e
                self.state = "failed"
                _notify("error", error=str(e), id=self.agent_id, type=self.agent_type)

        if self.fn_start:
            self._start_thread = threading.Thread(target=runner, daemon=True)
            self._start_thread.start()

        if not self._await_ready():
            # do not announce ready; let supervisor time out the start
            return

        if err_holder["err"] is None:
            self.state = "running"
            # Initialize QUIC connection (default to localhost loopback)
            try:
                addr = os.getenv("GAPI_MASTER_ADDR", "127.0.0.1:14242")
                adk.StartQUIC(addr)
            except Exception as e:
                print(f"[RUNNER] Warning: Failed to start QUIC client: {e}")

            _notify("ready", state=self.state, id=self.agent_id, type=self.agent_type)
            # Offload heartbeat to Go ADK
            adk.StartHeartbeat(self.agent_id, self.agent_type)

    def stop(self, timeout=10):
        try:
            if self.fn_stop:
                self.state = "stopping"
                _notify("stopping", id=self.agent_id, type=self.agent_type)
                with redirect_stdout(sys.stderr), redirect_stderr(sys.stderr):
                    self.fn_stop()
        finally:
            self.stop_evt.set()
            time.sleep(min(timeout, 0.25))
            try:
                # Go heartbeats stop automatically when process exits or we could explicitly stop it
                pass
            except Exception:
                pass

            try:
                if self._start_thread is not None and self._start_thread.is_alive():
                    self._start_thread.join(timeout=1.0)
            except Exception:
                pass
            self._start_thread = None

            self.state = "stopped"
            _notify("stopped", id=self.agent_id, type=self.agent_type)
            self.stop_evt = threading.Event()

    def reload(self):
        if self.fn_reload:
            _notify("reloading", id=self.agent_id, type=self.agent_type)
            with redirect_stdout(sys.stderr), redirect_stderr(sys.stderr):
                self.fn_reload()
            _notify("reloaded", id=self.agent_id, type=self.agent_type)
        else:
             # If no reload function, maybe we should warn? 
             # For now, success (no-op)
             pass

    def restart(self):
        if self.fn_restart:
            _notify("restarting", id=self.agent_id, type=self.agent_type)
            with redirect_stdout(sys.stderr), redirect_stderr(sys.stderr):
                self.fn_restart()
            _notify("restarted", id=self.agent_id, type=self.agent_type)
        else:
            # Fallback: stop() then start() is usually handled by the supervisor,
            # but if restart is called inside the runner, we might need to simulate it.
            # However, simpler agents rely on supervisor restart.
            # Here we just execute the hook if present.
            pass

def get_meta(mod, key, default=None):
    # Lookup using META_KEYS
    aliases = META_KEYS.get(key, [key])
    for k in aliases:
        v = getattr(mod, k, None)
        if v is not None:
            return v
    return default

def describe(mod, agent_id=None, agent_type=None) -> AgentMetadata:
    id_ = get_meta(mod, "id", agent_id or "unknown")
    name = get_meta(mod, "name", id_)
    ver  = get_meta(mod, "version", "0.0.1")
    typ  = get_meta(mod, "type", agent_type or "service")
    en   = get_meta(mod, "enabled", True)
    ivl  = get_meta(mod, "interval")
    desc = get_meta(mod, "desc", "")
    
    # Dependencies
    reqs = get_meta(mod, "requires", [])
    wants = get_meta(mod, "wants", [])
    wanted_by = get_meta(mod, "wanted_by", [])
    required_by = get_meta(mod, "required_by", [])
    
    # Sockets
    listen_stream = get_meta(mod, "listen_stream")

    caps = []
    # 1. Lifecycle methods explicitly present
    for key in ("initialize","init","setup","start","run","stop","shutdown","reload","restart"):
        if callable(getattr(mod, key, None)):
            caps.append(key)
    
    # 2. @capability decorated methods/functions
    # Scan all module level functions.
    #
    # The member name is deliberately NOT bound: this loop used to bind
    # `name`, which is also the local holding the agent's declared name a
    # few lines above, so every describe payload carried the last member
    # inspect.getmembers returned instead of the name (GAPI-DIV-081).
    for _, obj in inspect.getmembers(mod):
        if inspect.isfunction(obj) or inspect.ismethod(obj):
            declared = getattr(obj, "_gapi_capabilities", [])
            caps.extend(declared)
    
    # Deduplicate
    caps = list(dict.fromkeys(caps))
    
    # TypedDict construction
    describe_data = {
        "schema_version": SCHEMA_VERSION,
        "id": id_, "name": name, "version": ver, "type": typ, "language": "python",
        "enabled": en, "capabilities": caps, "description": desc,
        "requires": reqs, "wants": wants, "wanted_by": wanted_by, "required_by": required_by,
        "listen_stream": str(listen_stream) if listen_stream else "",
        "cpu_limit": str(get_meta(mod, "cpu_limit", "")),
        "memory_limit": str(get_meta(mod, "memory_limit", "")),
        "schedule": str(get_meta(mod, "schedule", "")),
    }
    if ivl is not None:
        describe_data["interval"] = ivl
        
    obj: AgentMetadata = {"describe": describe_data}
    return obj

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--module", required=True)
    ap.add_argument("--describe", action="store_true")
    ap.add_argument("--start", action="store_true")
    ap.add_argument("--stop", action="store_true")
    ap.add_argument("--reload", action="store_true")
    ap.add_argument("--restart", action="store_true")
    ap.add_argument("--id")
    # No default: an omitted --type must fall back to the module's own TYPE,
    # not to "service". TimerAgent.execute used to omit it, so a timer module
    # was run as a service and never terminated (GAPI-DIV-039).
    ap.add_argument("--type", default=None)
    args = ap.parse_args()

    try:
        mod = load_module(args.module)
    except Exception as e:
        print(f"[runner] failed to import module {args.module}: {e}", file=sys.stderr)
        traceback.print_exc(file=sys.stderr)
        return 2

    if args.describe:
        try:
            obj = describe(mod, args.id, args.type)
            sys.stdout.write(json.dumps(obj, ensure_ascii=False) + "\n"); sys.stdout.flush()
            return 0
        except Exception as e:
            print(f"[runner] describe failed: {e}", file=sys.stderr)
            traceback.print_exc(file=sys.stderr)
            return 3

    agent = AgentWrapper(
        mod,
        agent_id=(args.id or getattr(mod, "ID", None) or getattr(mod, "id", None) or "unknown"),
        agent_type=(args.type or str(get_meta(mod, "type", "service"))),
    )

    # Compute schema hash using native Go implementation
    schema_hash = "unknown"
    try:
        # We need the absolute path to the module file for hashing
        # The module spec should have it
        if getattr(mod, '__file__', None):
            schema_hash = adk.ComputeSchemaHash(mod.__file__)
            if not schema_hash:
                schema_hash = "unknown"
                print(f"[runner] Warning: Computed empty schema hash for {mod.__file__}", file=sys.stderr)
        else:
             print(f"[runner] Warning: Could not determine file path for module {args.module}, skipping hash", file=sys.stderr)
    except Exception as e:
        print(f"[runner] Warning: Failed to compute schema hash: {e}", file=sys.stderr)

    # Set the hash in the ADK
    adk.SetSchemaHash(schema_hash)

    # Initialize binding
    adk.Initialize(agent.agent_id, "1.0.0", agent.agent_type)


    def on_sigterm(signum, frame):
        agent.stop()
        # Give time for events to flush (especially over stdout/QUIC)
        time.sleep(5.0)
        sys.exit(0)

    signal.signal(signal.SIGTERM, on_sigterm)
    signal.signal(signal.SIGINT, on_sigterm)

    if args.start:
        agent.initialize()
        agent.start()
        # A service must outlive start() so the supervisor has a process to
        # signal. A one-shot must NOT: its caller waits for this process to
        # exit before scheduling the next fire.
        if not agent.oneshot:
            try:
                while True:
                    time.sleep(0.25)
            except KeyboardInterrupt:
                pass
    if args.reload:
        agent.reload()
    if args.restart:
        agent.restart()
    if args.stop:
        agent.stop()
    return 0

if __name__ == "__main__":
    sys.exit(main())
