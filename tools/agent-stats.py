#!/usr/bin/env python3
"""Token and tool-use statistics for Claude Code sessions run in this repository.

Reads the local transcripts (~/.claude/projects/<this repo>/*.jsonl and each session's subagents/) and prints
one line per session and per subagent, then totals, then a with/without comparison per skill. Use it to judge
whether the skills and the economy rules in AGENTS.md pay off: compare waves, or agents that used a skill
against agents that didn't.

Usage: tools/agent-stats.py [--branch BRANCH] [--since YYYY-MM-DD] [--session PREFIX] [--no-agents]
       tools/agent-stats.py --branch test/w1-unit      everything run for one wave

Columns: turns = model calls; peak = largest context of any call; new = uncached input tokens (input plus
cache writes); cached = cache reads; out = output tokens; res = characters of tool results fed back into
context, the part the economy rules control. Flags count known-wasteful calls:
  model-read  Read/Grep attempts on pkg/model/core (.claude/settings.json denies them; an attempt still
              means the agent ignored the model-shape skill)
  full-read   Read of a 400+ line file without limit
  verbose     go test -v without -run          cat        cat of a file or log
  raw-build   go build/vet run by hand (gopls reports compile errors)
"""
import argparse
import collections
import json
import pathlib
import re
import statistics

ROOT = pathlib.Path(__file__).resolve().parents[1]
PROJECTS = pathlib.Path.home() / ".claude" / "projects" / re.sub(r"[^A-Za-z0-9]", "-", str(ROOT))
# Slash commands count as skill use only when they name a skill, not a built-in such as /clear.
SKILL_NAMES = {p.name for d in (ROOT / ".claude" / "skills", pathlib.Path.home() / ".claude" / "skills")
               if d.is_dir() for p in d.iterdir()}


class Agent:
    def __init__(self, path, label):
        self.path, self.label = path, label
        self.start = self.model = ""
        self.usage = {}  # message id -> usage; a message split over several lines repeats its usage
        self.tools = collections.Counter()
        self.res_chars = collections.Counter()
        self.skills = collections.Counter()
        self.lsp = collections.Counter()
        self.flags = collections.Counter()
        self.children = []
        self.branches = set()

    def turns(self):
        return len(self.usage)

    def peak(self):
        return max((u.get("input_tokens", 0) + u.get("cache_read_input_tokens", 0)
                    + u.get("cache_creation_input_tokens", 0) for u in self.usage.values()), default=0)

    def total(self, *keys):
        return sum(u.get(k, 0) for u in self.usage.values() for k in keys)

    def res(self):
        return sum(self.res_chars.values())


def file_lines(path):
    try:
        with open(path, "rb") as f:
            return sum(1 for _ in f)
    except OSError:
        return 0


def flag_call(agent, name, inp):
    if name in ("Read", "Grep", "Glob"):
        target = inp.get("file_path") or inp.get("path") or ""
        if "pkg/model/core" in target:
            agent.flags["model-read"] += 1
        if name == "Read" and "limit" not in inp and file_lines(target) >= 400:
            agent.flags["full-read"] += 1
    elif name == "Bash":
        cmd = inp.get("command", "")
        if re.search(r"go test\b[^|;&]*\s-v\b", cmd) and "-run" not in cmd:
            agent.flags["verbose"] += 1
        if re.search(r"(^|[;&|]\s*)cat\s+[^<|]", cmd) and "<<" not in cmd:
            agent.flags["cat"] += 1
        if re.search(r"(^|[;&|]\s*)go (build|vet)\b", cmd):
            agent.flags["raw-build"] += 1
        for m in re.findall(r"make (?:-s )?([\w-]+)", cmd):
            agent.tools[f"make {m}"] += 1


def parse(path, label):
    agent = Agent(path, label)
    names = {}
    for line in path.open():
        try:
            d = json.loads(line)
        except ValueError:
            continue
        agent.start = agent.start or d.get("timestamp", "")
        if d.get("gitBranch"):
            agent.branches.add(d["gitBranch"])
        if d.get("type") == "ai-title":
            agent.label = f'{label} "{d.get("aiTitle", "")[:40]}"'
        msg = d.get("message") or {}
        content = msg.get("content")
        if d.get("type") == "assistant":
            if msg.get("usage") and msg.get("id"):
                agent.usage[msg["id"]] = msg["usage"]
                agent.model = msg.get("model", agent.model)
            for c in content or []:
                if c.get("type") != "tool_use":
                    continue
                name, inp = c["name"], c.get("input") or {}
                names[c["id"]] = name
                agent.tools[name] += 1
                if name == "Skill":
                    agent.skills[inp.get("skill", "?")] += 1
                if name == "LSP":
                    agent.lsp[inp.get("operation", "?")] += 1
                flag_call(agent, name, inp)
        elif d.get("type") == "user":
            if isinstance(content, str):
                for m in re.findall(r"<command-name>/?([\w:-]+)</command-name>", content):
                    if m in SKILL_NAMES or ":" in m:
                        agent.skills[m] += 1
                continue
            for c in content or []:
                if c.get("type") == "tool_result":
                    body = c.get("content")
                    size = len(body) if isinstance(body, str) else len(json.dumps(body or ""))
                    agent.res_chars[names.get(c.get("tool_use_id"), "?")] += size
    return agent


def k(n):
    return f"{n / 1e6:.1f}M" if n >= 1e6 else f"{n / 1e3:.0f}k" if n >= 1e3 else str(n)


def line(a, indent=""):
    top = ", ".join(f"{t} {n}" for t, n in a.tools.most_common(5))
    parts = [f"{indent}{a.start[:10]} {a.label}  {a.model.replace('claude-', '')}",
             f"turns={a.turns()} peak={k(a.peak())} new={k(a.total('input_tokens', 'cache_creation_input_tokens'))}"
             f" cached={k(a.total('cache_read_input_tokens'))} out={k(a.total('output_tokens'))} res={k(a.res())}",
             f"tools: {top}"]
    if a.skills:
        parts.append("skills: " + ", ".join(f"{s}×{n}" for s, n in a.skills.items()))
    if a.lsp:
        parts.append("lsp: " + ", ".join(f"{o}×{n}" for o, n in a.lsp.items()))
    if a.flags:
        parts.append("FLAGS: " + ", ".join(f"{f}×{n}" for f, n in a.flags.items()))
    return f"\n{' ' * len(indent)}    ".join(parts)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--since", default="")
    ap.add_argument("--session", default="")
    ap.add_argument("--no-agents", action="store_true")
    ap.add_argument("--branch", default="", help="only sessions that ran on this git branch")
    args = ap.parse_args()

    sessions = []
    for path in sorted(PROJECTS.glob("*.jsonl")):
        if not path.stem.startswith(args.session):
            continue
        s = parse(path, path.stem[:8])
        if s.start[:10] < args.since or not s.usage or (args.branch and args.branch not in s.branches):
            continue
        for sub in sorted((PROJECTS / path.stem / "subagents").glob("*.jsonl")):
            meta_path = sub.with_suffix(".meta.json")
            meta = json.loads(meta_path.read_text()) if meta_path.exists() else {}
            child = parse(sub, f'{sub.stem[6:12]} [{meta.get("agentType", "?")}] "{meta.get("description", "")[:40]}"')
            if child.usage:
                s.children.append(child)
        sessions.append(s)
    sessions.sort(key=lambda s: s.start)

    everyone = []
    for s in sessions:
        print(line(s))
        everyone.append(s)
        for c in s.children:
            everyone.append(c)
            if not args.no_agents:
                print(line(c, "  └ "))

    if not everyone:
        print(f"no transcripts in {PROJECTS}")
        return
    res = collections.Counter()
    for a in everyone:
        res.update(a.res_chars)
    print("\ntool results fed into context, by tool:", ", ".join(f"{t} {k(n)}" for t, n in res.most_common(8)))
    flags = collections.Counter()
    for a in everyone:
        flags.update(a.flags)
    print("wasteful calls:", ", ".join(f"{f}×{n}" for f, n in flags.items()) or "none")

    skills = sorted({s for a in everyone for s in a.skills})
    if skills:
        print("\nper skill, agents that used it vs agents that didn't (medians):")
        for sk in skills:
            used = [a for a in everyone if sk in a.skills]
            not_used = [a for a in everyone if sk not in a.skills]

            def med(group, f):
                return k(int(statistics.median(f(a) for a in group))) if group else "-"
            print(f"  {sk:15} used by {len(used):3}: turns {med(used, Agent.turns)}, peak {med(used, Agent.peak)},"
                  f" res {med(used, Agent.res)}   | others {len(not_used)}: turns {med(not_used, Agent.turns)},"
                  f" peak {med(not_used, Agent.peak)}, res {med(not_used, Agent.res)}")


if __name__ == "__main__":
    main()
