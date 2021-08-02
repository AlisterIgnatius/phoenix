package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"regexp"
	"strings"

	"github.com/AlisterIgnatius/phoenix/common"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/spaolacci/murmur3"
)

// ErrNotFound indicates that a requested resource does not exist in storage
type ErrNotFound struct {
	message string
}

func (e *ErrNotFound) Error() string {
	return e.message
}

// Store is a mockable interface corresponding to s3.S3.
type Store interface {
	PutObject(*s3.PutObjectInput) (*s3.PutObjectOutput, error)
	GetObject(*s3.GetObjectInput) (*s3.GetObjectOutput, error)
	DeleteObjects(*s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error)
}

// Repository provides read/write access to the Phoenix Content Repository.
type Repository struct {
	Store  Store
	Index  Index
	Bucket string
}

// Helper method for downloading files from S3.
func (r *Repository) get(key string) (*json.Decoder, error) {
	var input *s3.GetObjectInput
	var output *s3.GetObjectOutput
	var err error

	input = &s3.GetObjectInput{Bucket: aws.String(r.Bucket), Key: aws.String(key)}
	if output, err = r.Store.GetObject(input); err != nil {
		// Special-case a not found error, and return our own type (to simplify error handling for callers)
		var s3err awserr.Error
		if errors.As(err, &s3err) {
			if s3err.Code() == s3.ErrCodeNoSuchKey {
				return nil, &ErrNotFound{fmt.Sprintf("s3 resource: %s/%s not found", r.Bucket, key)}
			}
		}
		return nil, err
	}

	return json.NewDecoder(output.Body), nil
}

// Helper method for uploading files to S3.
func (r *Repository) put(key string, data []byte, meta map[string]*string) error {
	_, err := r.Store.PutObject(
		&s3.PutObjectInput{
			Body:        aws.ReadSeekCloser(bytes.NewReader(data)),
			Bucket:      aws.String(r.Bucket),
			Key:         aws.String(key),
			ContentType: aws.String("application/json"),
			Metadata:    meta,
		})

	return err
}

// Helper method for deleting files from S3.
func (r *Repository) delete(keys []string) error {
	var objects []*s3.ObjectIdentifier

	for _, key := range keys {
		objects = append(objects, &s3.ObjectIdentifier{Key: aws.String(key)})
	}

	_, err := r.Store.DeleteObjects(
		&s3.DeleteObjectsInput{
			Bucket: aws.String(r.Bucket),
			Delete: &s3.Delete{
				Objects: objects,
				Quiet:   aws.Bool(false),
			},
		})

	return err
}

// GetPage returns a Page by its ID
func (r *Repository) GetPage(id string) (*common.Page, error) {
	var data *json.Decoder
	var err error
	var page common.Page

	// Fetch
	if data, err = r.get(id); err != nil {
		return nil, fmt.Errorf("Error retrieving content: %w", err)
	}

	// Deserialize JSON
	if err = data.Decode(&page); err != nil {
		return nil, fmt.Errorf("Unable to deserialize JSON: %w", err)
	}

	return &page, nil
}

// GetPageByName returns a Page by its authority and name
func (r *Repository) GetPageByName(authority, name string) (*common.Page, error) {
	var id string
	var err error

	if id, err = r.Index.PageIDForName(authority, name); err != nil {
		return nil, err
	}

	return r.GetPage(id)
}

// GetNode returns a Node by its ID
func (r *Repository) GetNode(id string) (*common.Node, error) {
	var data *json.Decoder
	var err error
	var section common.Node

	// Fetch
	if data, err = r.get(id); err != nil {
		return nil, fmt.Errorf("Error retrieving content: %w", err)
	}

	// Deserialize JSON
	if err = data.Decode(&section); err != nil {
		return nil, fmt.Errorf("Unable to deserialize JSON: %w", err)
	}

	return &section, nil
}

// GetNodeByName returns a Node by its authority, a page name, and the node name
func (r *Repository) GetNodeByName(authority, pageName, name string) (*common.Node, error) {
	var id string
	var err error

	if id, err = r.Index.NodeIDForName(authority, pageName, name); err != nil {
		return nil, err
	}

	return r.GetNode(id)
}

// GetAbout returns an About by its ID
func (r *Repository) GetAbout(id string) (*common.Thing, error) {
	var data *json.Decoder
	var err error
	var about common.Thing

	// Fetch
	if data, err = r.get(id); err != nil {
		return nil, fmt.Errorf("Error retrieving content: %w", err)
	}

	// Deserialize JSON
	if err = data.Decode(&about); err != nil {
		return nil, fmt.Errorf("Unable to deserialize JSON: %w", err)
	}

	// Thing doesn't JSON serialize the ID
	about.ID = id

	return &about, nil
}

// GetTopics returns an array of RelatedTopics associated with a Node
func (r *Repository) GetTopics(node *common.Node) ([]common.RelatedTopic, error) {
	var data *json.Decoder
	var err error
	var topics []common.RelatedTopic

	// Fetch
	if data, err = r.get(topicsf(makeNodeID(node))); err != nil {
		return nil, fmt.Errorf("Unable to retrieve related topics for %s: %w", node.ID, err)
	}

	// Deserialize JSON
	if err = data.Decode(&topics); err != nil {
		return nil, fmt.Errorf("Failed to deserialize JSON: %w", err)
	}

	return topics, nil
}

// PutPage stores a Page.
func (r *Repository) PutPage(page *common.Page) error {
	var data []byte
	var err error

	if err = validatePage(page); err != nil {
		return err
	}

	if data, err = encodeJSON(page); err != nil {
		return err
	}

	metadata := map[string]*string{"type": aws.String("common.Page")}

	if err = r.put(page.ID, data, metadata); err != nil {
		return err
	}

	return nil
}

// PutBook stores Books as a source.
func (r *Repository) PutBook(book *common.Book) error {
	var data []byte
	var err error

	if data, err = encodeJSON(book); err != nil {
		return err
	}

	metadata := map[string]*string{"type": aws.String("common.Book")}

	if err = r.put(fmt.Sprintf("books/%s", book.Isbn), data, metadata); err != nil {
		return err
	}

	return nil
}

// PutNode stores a Node.
func (r *Repository) PutNode(node *common.Node) error {

	var data []byte
	var err error

	if err = validateNode(node); err != nil {
		return err
	}

	if data, err = encodeJSON(node); err != nil {
		return err
	}

	metadata := map[string]*string{"type": aws.String("common.Node")}

	if err = r.put(node.ID, data, metadata); err != nil {
		return err
	}

	return nil
}

// PutNode stores a Node Citations.
func (r *Repository) PutNodeCitations(id string, citation *[]common.Citation) error {
	var data []byte
	var err error

	if data, err = encodeJSON(citation); err != nil {
		return err
	}

	metadata := map[string]*string{"type": aws.String("common.Citations")}

	fmt.Println(id)

	if err = r.put(id, data, metadata); err != nil {
		return err
	}

	return nil
}

// PutPageCitation stores a Page Citation.
func (r *Repository) PutPageCitation(node *common.Node) error {
	var data []byte
	var err error

	if err = validateNode(node); err != nil {
		return err
	}

	if data, err = encodeJSON(node); err != nil {
		return err
	}

	metadata := map[string]*string{"type": aws.String("common.Node")}

	if err = r.put(node.ID, data, metadata); err != nil {
		return err
	}

	return nil
}

func (r *Repository) PutPageCitationEnhanced(citation *common.Citations) error {
	var data []byte
	var err error

	if data, err = encodeJSON(citation); err != nil {
		return err
	}

	metadata := map[string]*string{"type": aws.String("common.Citations")}

	if err = r.put(citation.IsPartOf[0], data, metadata); err != nil {
		return err
	}

	return nil
}

// PutAbout stores a Thing.  This method generates a unique ID and returns it on success; NOTE: If
// you assign an ID it will be overwritten.
func (r *Repository) PutAbout(thing *common.Thing) error {
	var data []byte
	var err error

	if data, err = encodeJSON(thing); err != nil {
		return err
	}

	metadata := map[string]*string{"type": aws.String("common.Thing")}

	if err = r.put(thing.ID, data, metadata); err != nil {
		return err
	}

	return nil
}

// PutTopics stores an array of RelatedTopic objects associated with a Node
func (r *Repository) PutTopics(node *common.Node, topics []common.RelatedTopic) error {
	var data []byte
	var err error
	var id = topicsf(makeNodeID(node))
	var metadata = map[string]*string{"type": aws.String("[]common.RelatedTopic")}

	if data, err = encodeJSON(topics); err != nil {
		return err
	}

	return r.put(id, data, metadata)
}

// DeletePage removes a Page from storage by its ID
func (r *Repository) DeletePage(id string) {
	// TODO: Do.
}

// DeleteNode removes a Node from storage by its ID
func (r *Repository) DeleteNode(id string) {
	// TODO: Do.
}

// DeleteAbout removes a Thing from storage by its ID
func (r *Repository) DeleteAbout(id string) {
	// TODO: Do.
}

// Update encapsulates the parts of a document involved in an update of the content repository.
type Update struct {
	Page                common.Page
	Citation            common.Node
	CitationEnhanced    common.Citations
	Nodes               []common.Node
	NodesCitations      []common.Citations
	Abouts              map[string]common.Thing
	PostPutNodeCallback func(common.Node) error
}

// Apply updates a document in the content repository.
func (r *Repository) Apply(update *Update) error {
	var err error

	// Baby steps: An argument could be made for breaking down the steps here into events that
	// trigger the respective actions, but we're not going there just yet.  An argument could
	// also be made for handling some of these steps concurrently (we could easily parallelize
	// uploads of Node & Things, for example), but we're not going there yet either.

	update.Page.HasPart = make([]string, 0)

	// Upload node objects.  Remember: the ordering of HasPart matters (keep this in mind
	// when/if adding concurrency at a later date).
	for i, node := range update.Nodes {
		var err error

		node.IsPartOf = []string{fmt.Sprintf("%s.json", update.Page.ID)}
		node.Source = update.Page.Source

		if err = r.PutNode(&node); err != nil {
			return fmt.Errorf("error storing node: %w", err)
		}

		update.Page.HasPart = append(update.Page.HasPart, fmt.Sprintf("%s.json", update.Nodes[i].ID))

		if update.PostPutNodeCallback != nil {
			// FIXME: Should we handle the error?  Ignore it?
			update.PostPutNodeCallback(node)
		}
	}

	update.Page.About = make(map[string]string)

	// Upload linked data objects.
	for k, v := range update.Abouts {
		var err error
		pagename := strings.ReplaceAll(update.Page.Name, " ", "_")
		v.ID = fmt.Sprintf("pages/%s/%s_about_%s", pagename, pagename, k)

		if err = r.PutAbout(&v); err != nil {
			return fmt.Errorf("error storing linked data object: %w", err)
		}

		update.Page.About[k] = v.ID
	}

	for _, citation := range update.NodesCitations {
		var err error

		if err = r.PutNodeCitations(fmt.Sprintf("%s_citations", citation.IsPartOf[0]), &citation.Citations); err != nil {
			return fmt.Errorf("error storing node: %w", err)
		}
	}

	// Put page citation
	if err = r.PutPageCitation(&update.Citation); err != nil {
		return err
	}

	// Put page citations enhanced
	if err = r.PutPageCitationEnhanced(&update.CitationEnhanced); err != nil {
		return err
	}

	// Overwrite the Page object
	if err = r.PutPage(&update.Page); err != nil {
		return err
	}

	// Perform indexing
	return r.Index.Apply(update)
}

var (
	// Regular expression that matches UUIDs
	tidRegexp = regexp.MustCompile("[A-Za-z0-9]{8}-[A-Za-z0-9]{4}-[A-Za-z0-9]{4}-[A-Za-z0-9]{4}-[A-Za-z0-9]{12}")
)

// Helpers are helpful.
func encodeJSON(v interface{}) ([]byte, error) {
	var buffer *bytes.Buffer
	var encoder *json.Encoder
	var err error

	buffer = bytes.NewBuffer(make([]byte, 0, 0))
	encoder = json.NewEncoder(buffer)

	// Don't escape HTML (we store raw HTML in attributes)
	encoder.SetEscapeHTML(false)

	if err = encoder.Encode(v); err != nil {
		return nil, err
	}

	return buffer.Bytes(), err
}

func validateSource(source *common.Source) error {
	fmt.Println(fmt.Sprintf("SOURCE ID: %d", source.ID))
	if source.ID <= 0 {
		return fmt.Errorf("uninitialized common.Source.ID attribute (+%v)", source)
	}
	if source.Revision <= 0 {
		return fmt.Errorf("uninitialized common.Source.Revision attribute (+%v)", source)
	}
	if !tidRegexp.Match([]byte(source.TimeUUID)) {
		return fmt.Errorf("invalid common.Source.TimeUUID attribute (+%v)", source)
	}
	if source.Authority == "" {
		return fmt.Errorf("uninitialized common.Source.Authority attribute (+%v)", source)
	}
	return nil
}

func validatePage(page *common.Page) error {
	fmt.Println(fmt.Sprintf("PAGE SOURCE ID: %d", page.Source.ID))

	if page.Name == "" {
		return fmt.Errorf("uninitialized page.Name attribute (%+v)", page)
	}
	if page.URL == "" {
		return fmt.Errorf("uninitialized page.URL attribute (%+v)", page)
	}
	if page.DateModified.IsZero() {
		return fmt.Errorf("uninitialized page.DateModified attribute (%+v)", page)
	}
	if len(page.HasPart) < 1 {
		return fmt.Errorf("zero-length page.HasPart attribute (%+v)", page)
	}

	// return validateSource(&page.Source)
	// We might not need Source in the future.
	return nil
}

func validateNode(node *common.Node) error {
	if node.DateModified.IsZero() {
		return fmt.Errorf("uninitialized node.DateModified attribute (%+v)", node)
	}
	return validateSource(&node.Source)
}

func makeRandomID() string {
	return uuid.New().String()
}

func newHash64() hash.Hash64 {
	return murmur3.New64()
}

func asHex(v uint64) string {
	return fmt.Sprintf("%x", v)
}

// To maintain page ID stability, we fake a globally unique (and opaque) ID using a hash of
// the underlying wiki and page ID.
func makePageID(page *common.Page) string {
	hasher := newHash64()
	hasher.Write([]byte(fmt.Sprintf("%s-%d", page.Source.Authority, page.Source.ID)))
	return asHex(hasher.Sum64())
}

// To maintain node ID stability, we create a globally unique (and opaque) ID from a hash of
// the underlying wiki, page ID, and node name.
func makeNodeID(node *common.Node) string {
	hasher := newHash64()
	hasher.Write([]byte(fmt.Sprintf("%s-%d-%s", node.Source.Authority, node.Source.ID, node.Name)))
	return asHex(hasher.Sum64())
}

// Return formatted keys for page, node, and data objects.
func topicsf(id string) string {
	return fmt.Sprintf("/topics/%s", id)
}
