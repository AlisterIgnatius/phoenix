package main

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/wikimedia/phoenix/common"
)

var (
	ignoredNodes = map[string]bool{
		"References": true,
	}
)

func getSectionName(section *goquery.Selection) string {
	return section.Find("h2").First().Text()
}

func parseParsoidDocumentNodes(document *goquery.Document, page *common.Page) ([]common.Node, error) {
	var err error
	var modified = page.DateModified
	var nodes = make([]common.Node, 0)

	sections := document.Find("html>body>section[data-mw-section-id]")
	for i := range sections.Nodes {
		section := sections.Eq(i)

		node := common.Node{}
		node.Source = page.Source
		node.Name = getSectionName(section)
		node.DateModified = modified

		if val, ok := ignoredNodes[node.Name]; ok && val {
			continue
		}

		if node.Unsafe, err = section.Html(); err != nil {
			return []common.Node{}, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}
