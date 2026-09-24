#!/usr/bin/env python3
"""Build a self-contained subagent prompt for one task of a modernization wave.

Usage: task_prompt.py <wave> <task> [<task> ...]
       task_prompt.py w1 U1 U2 U3        one prompt per task, separated by "=====" lines
       task_prompt.py w4 S1

The task's table row (with the table header) or its "### <task>" section is pasted in verbatim, so the
subagent never opens the wave file. The first line of each prompt, starting with "#", is for the coordinator:
the Agent tool `model` to use. Anything the script can't infer is left as <FILL: ...>; fill it in before
dispatching.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[3]
TIERS = {"cheap": "haiku", "mid": "sonnet", "strong": "opus", "coordinator": "coordinator",
         "haiku": "haiku", "sonnet": "sonnet", "opus": "opus"}

TEMPLATE = """\
You are adding tests to pcom. Read docs/testing.md; read no other docs.
Task: {task}.

{excerpt}

You own exactly these files: {owns}. Do not edit any other file.
Do not change production code. Do not run git. Do not edit go.mod.
Navigate with the LSP tool (load it with ToolSearch "select:LSP"); get model shapes with the model-shape
skill (`make model T=<Model>`); never read pkg/model/core. On a failing test, follow the test-failure skill.
Create fixtures with the test factories. If a helper is missing, stop and report exactly what you need rather
than writing ORM calls in your test.
If you find a bug: write the test for correct behavior, add t.Skip("known bug: <describe>"), and put a
one-line repro in your report.
After editing, check compilation with `make vet-q PKG={pkg}` (language-server diagnostics don't reach you).
Test only with `make test-q PKG={pkg}`, `make cover-q PKG={pkg}` and `go test <pkg> -run <Test> -count=1`.
Done when: tests pass and {done}.
Reply in at most 12 lines, in exactly this format, with no code and no logs:
DONE | BLOCKED  <task id>
files: <paths created or changed>
coverage: <pkg> <n>%
bugs: <test name> - <one-line repro>   (one per line, or "none")
needs: <missing factory helper / code change>   (or "none")"""


def cells(line):
    return [c.strip() for c in line.strip().strip("|").split("|")]


def from_table(lines, task):
    """Return (excerpt, fields) for a table row whose first cell starts with the task id."""
    header = None
    for i, line in enumerate(lines):
        if not line.startswith("|"):
            header = None
            continue
        if header is None:
            header = (line, lines[i + 1] if i + 1 < len(lines) else "")
            continue
        if line == header[1]:
            continue
        first = cells(line)[0]
        if re.match(rf"\**{re.escape(task)}\b", first):
            fields = dict(zip([h.lower() for h in cells(header[0])], cells(line)))
            return "\n".join([header[0], header[1], line]), fields
    return None, {}


def from_section(lines, task):
    """Return the '### <task> ...' section, up to the next heading of the same or a higher level."""
    for i, line in enumerate(lines):
        m = re.match(r"(#+) ", line)
        if m and re.match(rf"#+ {re.escape(task)}\b", line):
            level = len(m.group(1))
            out = [line]
            for nxt in lines[i + 1:]:
                h = re.match(r"(#+) ", nxt)
                if h and len(h.group(1)) <= level:
                    break
                out.append(nxt)
            return "\n".join(out).strip()
    return None


def from_paragraph(text, task):
    paras = [p for p in text.split("\n\n") if f"**{task}**" in p]
    return "\n\n".join(paras) or None


def build(wave, task):
    path = ROOT / "docs" / "plan" / f"{wave.lower()}.md"
    text = path.read_text()
    lines = text.splitlines()
    excerpt, fields = from_table(lines, task)
    if excerpt is None:
        excerpt = from_section(lines, task) or from_paragraph(text, task)
    if excerpt is None:
        sys.exit(f"task {task} not found in {path.relative_to(ROOT)}")

    tier = (fields.get("tier") or fields.get("model") or "").strip("`* ").lower()
    if not tier:
        m = re.search(r"\b(cheap|mid|strong)\b", excerpt)
        tier = m.group(1) if m else ""
    where = next((v for k, v in fields.items() if k.startswith("package")), excerpt)
    pkgs = re.findall(r"`((?:pkg|cmd|e2e)/[\w/.-]+)`", where)
    dirs = [p.rsplit("/", 1)[0] if p.endswith(".go") else p.rstrip("/") for p in pkgs]
    pkg = " ".join(f"./{d}/..." for d in dict.fromkeys(dirs)) or "<FILL: ./pkg/x/...>"
    target = fields.get("target")
    if not target:
        m = re.search(r"target (\d+%)", excerpt)
        target = m.group(1) if m else None
    done = f"the coverage of the task's packages is at least {target}" if target else \
        "<FILL: the task's done-when, from the excerpt>"
    return "\n".join([
        f"# model: {TIERS.get(tier, '<FILL: tier>')}   ({wave.upper()}.{task}, tier {tier or '?'})",
        TEMPLATE.format(task=f"{wave.upper()}.{task}", excerpt=excerpt, pkg=pkg, done=done,
                        owns="<FILL: new _test.go files and testdata/ in " + (pkg if pkgs else "the task's packages") + ">"),
    ])


if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    print("\n=====\n".join(build(sys.argv[1], t) for t in sys.argv[2:]))
