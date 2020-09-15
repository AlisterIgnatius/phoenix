#!/bin/bash

set -e

query()
{
    cat <<EOF
{
  "query":
    "{
       page(name: { authority: \"simple.wikipedia.org\", name: \"$1\"} ) {
         name
         dateModified
         hasPart(limit: 3, offset: 2) {
           id
           name
           dateModified
         }
         about {
           key
           val
         }
       }
    }"
}
EOF
}

echo "Query (JSON-encoded) -------------"
query "$1"

echo

echo "Response -------------------------"
query "$1" | curl -XPOST localhost:8080/query -d @- 2>/dev/null | json_pp
