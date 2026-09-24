#!/usr/bin/env bash
# Print the shape of a generated sqlboiler model compactly, so that nobody needs to
# read pkg/model/core (1 MB of generated code) or the tag-heavy `go doc` output.
#
# Usage: tools/model.sh            list the model types
#        tools/model.sh User       fields, relationships, finders and methods of User
set -eu

pkg=./pkg/model/core

if [ $# -eq 0 ]; then
	go doc -short "$pkg" | sed -n 's/^type \([A-Za-z]*\) struct.*/\1/p' |
		grep -Ev 'Slice$|Hook$|Query$|Where$|Columns$|Rels$|Table$|^[a-z]' | tr '\n' ' '
	echo
	exit 0
fi

t=$1
lower="$(printf '%s' "${t:0:1}" | tr '[:upper:]' '[:lower:]')${t:1}"
strip_tags() { sed -E 's/[[:space:]]*`[^`]*`//; s/[[:space:]]+/ /g; s/^ //'; }

echo "== $t fields"
go doc "$pkg" "$t" | sed -n '/^type/,/^}/p' | sed '1d;$d' | grep -Ev '^\s*(R|L) ' | strip_tags | grep -v '^$'

echo "== $t relationships (o.R.<Name>, load with qm.Load(\"<Name>\"))"
go doc -u "$pkg" "${lower}R" 2>/dev/null | sed -n '/^type/,/^}/p' | sed '1d;$d' | strip_tags

echo "== query helpers: ${t}Where.<Field>.EQ(v), ${t}Columns.<Field>, ${t}Rels.<Name>"
go doc -short "$pkg" | grep -E "^func [A-Za-z]*\(" | grep -E "\) \*?${lower}Query$" | sed 's/^func //'

echo "== funcs and methods (P variants omitted)"
go doc "$pkg" "$t" | grep -E '^func ' | grep -Ev 'P\(' |
	sed -E 's/\(ctx context\.Context, exec boil\.ContextExecutor,? ?/(ctx, exec, /; s/^func //'
