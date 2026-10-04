#!/usr/bin/env python3
"""Build a self-contained subagent prompt for one task of a modernization wave.

Usage: task_prompt.py <wave> <task> [<task> ...]
       task_prompt.py r7 L1 L2 L3        one prompt per task, separated by "=====" lines

The task's "### <task>" section and its table row (with the table header) are pasted in verbatim, so the
subagent never opens the wave file. The section holds the spec; the row adds the tier and ownership. A wave
written as bold bullets (`- **Step 1** ...`) gets its intro, the task's bullet and the non-task bullets
(`**Invariant**`), not the sibling steps. The first line of each prompt, starting with "#", is for the coordinator:
the Agent tool `model` to use. Anything the script can't infer is left as <FILL: ...>; fill it in before
dispatching.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[3]
TIERS = {"cheap": "haiku", "mid": "sonnet", "strong": "opus", "coordinator": "coordinator",
         "haiku": "haiku", "sonnet": "sonnet", "opus": "opus"}

REPORT = """\
Reply in at most 12 lines, in exactly this format, with no code and no logs:
DONE | BLOCKED  <task id>
files: <paths created or changed>
coverage: <pkg> <n>%
bugs: <test name> - <one-line repro>   (one per line, or "none")
needs: <missing factory helper / code change>   (or "none")"""

# Refactor waves (R*) change production code but not behavior.
REFACTOR_TEMPLATE = """\
You are refactoring pcom. Read docs/architecture.md and docs/testing.md; read no other docs.
Task: {task}.

{excerpt}

You own exactly these files: {owns}. Do not edit any other file; if another file must change, stop and
report it under needs:. Do not run git.
Navigate with the LSP tool (load it with ToolSearch "select:LSP"); get model shapes with the model-shape
skill (`make model T=<Model>`); never read pkg/model/core. On a failing test, follow the test-failure skill.
Never cat a whole file: grep -n or LSP documentSymbol first, then Read only the lines you need.
A bug you notice outside your task is not yours to fix: report it under bugs: with a one-line repro.
Behavior must not change, failure paths included: a panic stays a panic and an error keeps its message.
The E2E tests (e2e/), the browser tests (e2e/browser) and the seed crawl are not edited. A test you move may change its call site but not its assertions. If an assertion has to change,
stop and report it.
{layering}
After editing, check compilation with `make vet-q PKG={pkg}` (language-server diagnostics don't reach you).
Test with `make test-q PKG={pkg}`, then `make test-q PKG=./e2e/...` once at the end.
Before reporting, run `make fix-q PKG={pkg}` (CI's Go Fix job commits whatever go fix rewrites), then
`make lint-q PKG={pkg}` and the tests again. Report only when all three are clean.
Done when: tests pass and {done}.
{report}"""

LAYERING = """\
Layering: handlers bind input, call one service method and render; services hold rules, authorization
and transactions; every query lives in pkg/repo. Never add an entry to the pkg/arch allowlist."""


# Feature waves (F*) change behavior on purpose, as the wave file decides; the new tests prove the feature.
FEATURE_TEMPLATE = """\
You are building a feature in pcom. Read docs/architecture.md and docs/testing.md; read no other docs.
Task: {task}.

{excerpt}

You own exactly these files: {owns}. Do not edit any other file; if another file must change, stop and
report it under needs:. Do not run git.
Navigate with the LSP tool (load it with ToolSearch "select:LSP"); get model shapes with the model-shape
skill (`make model T=<Model>`); never read pkg/model/core. On a failing test, follow the test-failure skill.
Before touching cmd/web/client or an htmx handler, follow the frontend-htmx skill.
Never cat a whole file: grep -n or LSP documentSymbol first, then Read only the lines you need.
A bug you notice outside your task is not yours to fix: report it under bugs: with a one-line repro.
Build exactly what the task describes; every decision in it is made, so don't reopen one. If something is
undecided, stop and report it under needs:.
Layering: handlers bind input, call one service method and render; services hold rules and authorization;
every query lives in pkg/repo. Never grow the pkg/arch allowlist.
A task that changes what a user sees names the guide page it affects under needs:, so the wave's docs task updates it.
Tests prove what the feature is for, not incidental markup: what a user does in a page is a browser test
(e2e/browser), a server rule is an E2E or service test. A rule a service test already owns is not
repeated in E2E: a route test proves the wiring with one case the rule admits and one it rejects. An
existing assertion may change only where the task changes that behavior; name each one under bugs:. Keep
tests compact (ground rule 7 in docs/testing.md).
After editing, check compilation with `make vet-q PKG={pkg}` (language-server diagnostics don't reach you).
Test with `make test-q PKG={pkg}`, then `make test-q PKG=./e2e/...` once at the end{ui}.
Before reporting, run `make fix-q PKG={pkg}` (CI's Go Fix job commits whatever go fix rewrites), then
`make lint-q PKG={pkg}` and the tests again. Report only when all three are clean.
Done when: {done}.
{report}"""

BROWSER_RUN = (", and your browser tests with `tools/qrun.sh test-ui go test -tags browser -count=1 -run"
               " '<tests>' ./e2e/browser/...` (not `make test-ui`: it rebuilds the assets other tasks share). If you"
               " change cmd/web/client/js or scss, rebuild once with `tools/qrun.sh ui-build yarn --cwd cmd/web build`"
               " and say so under needs:")


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


def bullets(lines):
    """Yield (label, lines) for each top-level '- **Label** ...' bullet, continuation lines included."""
    item = None
    for line in lines:
        m = re.match(r"- \*\*(.+?)\*\*", line)
        if m or (item and (not line.strip() or not line.startswith("  "))):
            if item:
                yield item
            item = (m.group(1).rstrip(":"), [line]) if m else None
        elif item:
            item[1].append(line)
    if item:
        yield item


def from_bullet(lines, task):
    """For waves written as '- **Step 1** ...' bullets: the wave's intro (everything before the first bullet),
    the task's bullet, and the bullets that aren't sibling tasks (an **Invariant**, say)."""
    items = list(bullets(lines))
    if not any(label == task for label, _ in items):
        return None
    sibling = re.compile(re.sub(r"\d+", r"\\d+", re.escape(task)) + "$")
    first = next(i for i, line in enumerate(lines) if re.match(r"- \*\*", line))
    intro = "\n".join(lines[:first]).strip()
    keep = ["\n".join(body).rstrip() for label, body in items if label == task or not sibling.match(label)]
    own = next("\n".join(body) for label, body in items if label == task)
    return "\n\n".join([intro, "\n".join(keep)]), own


def from_paragraph(text, task):
    paras = [p for p in text.split("\n\n") if f"**{task}**" in p]
    return "\n\n".join(paras) or None


def build(wave, task):
    path = ROOT / "docs" / "plan" / f"{wave.lower()}.md"
    text = path.read_text()
    lines = text.splitlines()
    row, fields = from_table(lines, task)
    section = from_section(lines, task)
    excerpt = "\n\n".join(x for x in (section, row) if x)
    own = None  # for a bullet task, only its own bullet names its packages, not the wave's intro
    if not excerpt:
        excerpt, own = from_bullet(lines, task) or (from_paragraph(text, task), None)
    if excerpt is None:
        sys.exit(f"task {task} not found in {path.relative_to(ROOT)}")

    tier = (fields.get("tier") or fields.get("model") or "").lower()
    m = re.search(r"\b(" + "|".join(TIERS) + r")\b", tier)
    tier = m.group(1) if m else tier.strip("`* ")
    if not tier:
        m = re.search(r"\b(cheap|mid|strong)\b", excerpt)
        tier = m.group(1) if m else ""
    m = re.search(r"\bOwns (?!\|)(.+?)\.(?=\s|\||$)", excerpt, re.S)  # a sentence, not the table header
    owned = m.group(1).strip() if m else fields.get("owns")
    where = owned or next((v for k, v in fields.items() if k.startswith("package")), own or excerpt)
    pkgs = [p for p in re.findall(r"`((?:pkg|cmd|e2e)/[\w/.-]+)`", where)
            if not p.startswith("cmd/web/client") and not re.search(r"\.(?!go$)\w+$", p)]
    dirs = [p.rsplit("/", 1)[0] if p.endswith(".go") else p.rstrip("/") for p in pkgs]
    dirs = [d for d in dict.fromkeys(dirs) if not any(d.startswith(o + "/") for o in dirs)]
    pkg = " ".join(f"./{d}/..." for d in dirs) or "<FILL: ./pkg/x/...>"
    if len(dirs) > 1:
        pkg = f'"{pkg}"'  # make would read a second unquoted path as a target
    target = fields.get("target")
    if not target:
        m = re.search(r"target (\d+%)", excerpt)
        target = m.group(1) if m else None
    done = f"the coverage of the task's packages is at least {target}" if target else \
        fields.get("done when") or "<FILL: the task's done-when, from the excerpt>"
    header = f"# model: {TIERS.get(tier, '<FILL: tier>')}   ({wave.upper()}.{task}, tier {tier or '?'})"
    if wave.lower().startswith("f"):
        dirs = [d for d in dirs if not d.startswith("e2e")]  # the preamble runs e2e once at the end
        pkg = " ".join(f"./{d}/..." for d in dirs) or "<FILL: ./pkg/x/...>"
        pkg = f'"{pkg}"' if len(dirs) > 1 else pkg
        ui = BROWSER_RUN if "browser" in excerpt else ""
        body = FEATURE_TEMPLATE.format(task=f"{wave.upper()}.{task}", excerpt=excerpt, pkg=pkg, done=done,
                                       report=REPORT, ui=ui, owns=owned or "<FILL: the task's owned files>")
    elif wave.lower().startswith("r"):
        dirs = [d for d in dirs if not d.startswith("e2e")]  # the preamble runs e2e once at the end
        pkg = " ".join(f"./{d}/..." for d in dirs) or "<FILL: ./pkg/x/...>"
        pkg = f'"{pkg}"' if len(dirs) > 1 else pkg
        body = REFACTOR_TEMPLATE.format(task=f"{wave.upper()}.{task}", excerpt=excerpt, pkg=pkg, done=done,
                                        report=REPORT, owns=owned or "<FILL: the task's owned files>",
                                        layering=LAYERING)
    else:
        sys.exit(f"{wave}: only R- and F-waves have a preamble")
    return "\n".join([header, body])


if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    print("\n=====\n".join(build(sys.argv[1], t) for t in sys.argv[2:]))
