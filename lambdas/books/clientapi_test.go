package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testAuthor = []string{"Fujia Yang", "Joseph H. Hamilton"}
var testPage = "test_page"
var handlerTestWikitext = fmt.Sprintf(`
{
	"parse": {
		"title": "Albert Einstein",
		"pageid": 2138,
		"wikitext": "%s"
	}
}`, testBook)

var handlerTestCitation = `[
  {
    "itemType": "book",
    "title": "The Oxford companion to United States history",
    "oclc": "44426920",
    "url": "https://www.worldcat.org/oclc/44426920",
    "ISBN": [
      "0-19-508209-5",
      "978-0-19-508209-8",
      "978-0-19-989109-2",
      "0-19-989109-5"
    ],
    "edition": "First edition",
    "place": "Oxford",
    "date": "2001",
    "numPages": "xliv, 940 pages",
    "abstractNote": "The Oxford Companion to United States History covers everything from Jamestown and the Puritans to the Human Genome Project and the Internet. Written in clear, graceful prose for researchers, browsers, and general readers alike, this is the volume that addresses the totality of the American experience, its triumphs and heroes as well as its tragedies and darker moments",
    "contributor": [
      [
        "Eric H.",
        "Monkkonen"
      ],
      [
        "Ronald L.",
        "Numbers"
      ],
      [
        "David M.",
        "Oshinsky"
      ],
      [
        "Emily S.",
        "Rosenberg"
      ],
      [
        "Paul S.",
        "Boyer"
      ],
      [
        "Melvyn",
        "Dubofsky"
      ]
    ],
    "accessDate": "2021-06-09",
    "source": [
      "WorldCat"
    ]
  }
]`

func getWikitextTestHandler(rw http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["page"]

	if !ok || len(keys[0]) < 1 {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	page := keys[0]

	if page != testPage {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	_, _ = rw.Write([]byte(handlerTestWikitext))
}

func getCitationTestHandler(rw http.ResponseWriter, r *http.Request) {
	_, _ = rw.Write([]byte(handlerTestCitation))
}

func badReqTestHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusBadRequest)
}

const (
	testWiki     = "/w/api.php"
	testCitation = "/api/rest_v1/data/citation/mediawiki/test_isbn"
)

func createTestServer() http.Handler {
	srv := http.NewServeMux()

	srv.HandleFunc(testWiki, getWikitextTestHandler)
	srv.HandleFunc(testCitation, getCitationTestHandler)

	return srv
}

func TestClient(t *testing.T) {
	assert := assert.New(t)
	srv := httptest.NewServer(createTestServer())
	defer srv.Close()

	client := NewClient(srv.URL)

	t.Run("get wikitext", func(t *testing.T) {
		data, err := client.GetWikitext(testPage)

		assert.NoError(err)
		assert.Equal(testBook, data)
	})

	t.Run("get citation", func(t *testing.T) {
		testCitoidBook := &CitoidBook{
			Numpages: "xliv, 940 pages",
		}

		data, err := client.GetCitoidBook("test_isbn")

		assert.NoError(err)
		assert.Equal(testCitoidBook.Numpages, data.Numpages)
	})

	t.Run("get citation fail", func(t *testing.T) {
		_, err := client.GetWikitext("not_found")

		assert.Error(err)
	})
}
