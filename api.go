package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Endpoints from https://buzzheavier.com/developers. The environment overrides
// exist so the client can be pointed at a local mock server in tests.
var (
	siteBase   = envOr("BUZZHEAVIER_SITE_URL", "https://buzzheavier.com")
	apiBase    = envOr("BUZZHEAVIER_API_URL", "https://buzzheavier.com/api")
	uploadBase = envOr("BUZZHEAVIER_UPLOAD_URL", "https://w.buzzheavier.com")
)

const (
	maxNameLen = 500
	maxNoteLen = 500
	apiTimeout = 30 * time.Second
)

func envOr(key, def string) string {
	if v := strings.TrimRight(os.Getenv(key), "/"); v != "" {
		return v
	}
	return def
}

// Item is a file or directory as returned by the upload and file manager APIs.
type Item struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	IsDirectory bool       `json:"isDirectory"`
	ParentID    string     `json:"parentId"`
	LocationID  string     `json:"locationId"`
	Size        int64      `json:"size"`
	Expiry      *time.Time `json:"expiry"`
	Views       int64      `json:"views"`
	Downloads   int64      `json:"downloads"`
	Note        string     `json:"note"`
	CreatedAt   *time.Time `json:"createdAt"`
	Children    []Item     `json:"children"`
	NextCursor  string     `json:"nextCursor"`
}

// Link is the public download page for an uploaded file.
func (it *Item) Link() string { return siteBase + "/" + it.ID }

type Location struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// APIError is a non-success answer from Buzzheavier.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	switch {
	case e.Status == http.StatusUnauthorized:
		return "unauthorized: Buzzheavier rejected the Account ID (check it in Settings)"
	case e.Status == http.StatusNotFound && (e.Message == "" || strings.EqualFold(e.Message, "not found")):
		return "not found: no file or folder with that ID"
	case e.Message != "":
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
	default:
		return fmt.Sprintf("HTTP %d %s", e.Status, http.StatusText(e.Status))
	}
}

func isUnauthorized(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusUnauthorized
}

// isNameConflict reports whether the server refused an upload or rename
// because the target folder already holds something with that name.
func isNameConflict(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && strings.Contains(strings.ToLower(ae.Message), "already exist")
}

type envelope struct {
	Code  int             `json:"code"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

type Client struct {
	http  *http.Client
	token string
}

func newClient(token string) *Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSHandshakeTimeout = 20 * time.Second
	return &Client{http: &http.Client{Transport: t}, token: strings.TrimSpace(token)}
}

func (c *Client) newRequest(ctx context.Context, method, rawURL string, body io.Reader, auth bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "BuzzheavierClient/"+version+" (+"+repoURL+")")
	req.Header.Set("Accept", "application/json")
	if auth && c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// decode reads a Buzzheavier JSON envelope and unpacks its data into out.
func decode(resp *http.Response, out any) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	var env envelope
	jsonErr := json.Unmarshal(raw, &env)
	if resp.StatusCode >= 300 || (jsonErr == nil && env.Error != "") {
		msg := env.Error
		if jsonErr != nil {
			msg = strings.TrimSpace(string(raw))
			if len(msg) > 200 || strings.HasPrefix(msg, "<") {
				msg = "" // an HTML error page tells the user nothing useful
			}
		}
		status := resp.StatusCode
		if status < 300 && env.Code >= 300 {
			status = env.Code
		}
		return &APIError{Status: status, Message: msg}
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if jsonErr != nil {
		return fmt.Errorf("unexpected response from server: %.120s", strings.TrimSpace(string(raw)))
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("unexpected response from server: %w", err)
	}
	return nil
}

func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := c.newRequest(ctx, method, apiBase+path, rd, true)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return friendlyNetErr(err)
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func friendlyNetErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return errCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("the server took too long to answer, please try again")
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("could not reach Buzzheavier: %v", ue.Err)
	}
	return err
}

var errCancelled = errors.New("cancelled")

// ---- Public / account endpoints ----

func (c *Client) Locations(ctx context.Context) ([]Location, error) {
	var locs []Location
	return locs, c.call(ctx, http.MethodGet, "/locations", nil, &locs)
}

func (c *Client) Account(ctx context.Context) (map[string]any, error) {
	var acc map[string]any
	return acc, c.call(ctx, http.MethodGet, "/account", nil, &acc)
}

// ---- File manager ----

func (c *Client) Root(ctx context.Context) (*Item, error) {
	it, err := c.list(ctx, "/fs")
	if err != nil {
		return nil, err
	}
	if it.ID == "" {
		return nil, errors.New("the server did not return a root folder ID")
	}
	return it, nil
}

// Get returns a file, or a folder with all of its children.
func (c *Client) Get(ctx context.Context, id string) (*Item, error) {
	return c.list(ctx, "/fs/"+url.PathEscape(id))
}

// list follows nextCursor so large folders come back complete.
func (c *Client) list(ctx context.Context, path string) (*Item, error) {
	var it Item
	if err := c.call(ctx, http.MethodGet, path, nil, &it); err != nil {
		return nil, err
	}
	for page := 0; it.NextCursor != "" && page < 1000; page++ {
		var next Item
		if err := c.call(ctx, http.MethodGet, path+"?cursor="+url.QueryEscape(it.NextCursor), nil, &next); err != nil {
			return nil, err
		}
		it.Children = append(it.Children, next.Children...)
		it.NextCursor = next.NextCursor
	}
	return &it, nil
}

func (c *Client) CreateDir(ctx context.Context, parentID, name string) (*Item, error) {
	var it Item
	err := c.call(ctx, http.MethodPost, "/fs/"+url.PathEscape(parentID), map[string]string{"name": name}, &it)
	return &it, err
}

func (c *Client) Rename(ctx context.Context, id, name string) error {
	return c.call(ctx, http.MethodPatch, "/fs/"+url.PathEscape(id), map[string]string{"name": name}, nil)
}

// The developer docs list PUT for moving and for notes, but the live API
// answers 405 to PUT on /fs/{id}; PATCH accepts name, parentId and note.

func (c *Client) Move(ctx context.Context, id, parentID string) error {
	if id == parentID {
		return errors.New("a folder cannot be moved into itself")
	}
	return c.call(ctx, http.MethodPatch, "/fs/"+url.PathEscape(id), map[string]string{"parentId": parentID}, nil)
}

// SetNote changes a file's note. Buzzheavier ignores empty notes, so a note
// can be replaced but not removed.
func (c *Client) SetNote(ctx context.Context, id, note string) error {
	if strings.TrimSpace(note) == "" {
		return errors.New("the note is empty")
	}
	return c.call(ctx, http.MethodPatch, "/fs/"+url.PathEscape(id), map[string]string{"note": note}, nil)
}

func (c *Client) Delete(ctx context.Context, id string) error {
	return c.call(ctx, http.MethodDelete, "/fs/"+url.PathEscape(id), nil, nil)
}

// ---- Upload ----

type UploadOptions struct {
	Name       string
	ParentID   string // account uploads only; must be a real folder ID
	LocationID string
	Note       string
}

func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("file name is empty")
	}
	if n := utf8.RuneCountInString(name); n > maxNameLen {
		return fmt.Errorf("file name is %d characters long, Buzzheavier allows at most %d", n, maxNameLen)
	}
	return nil
}

// sanitizeName replaces characters Buzzheavier refuses in file names
// ("the Name field is invalid") or silently mangles (backslash).
func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '#' || r == ';' || r == '|' || r == '/' || r == '\\':
			return '_'
		case r < 0x20 || r == 0x7f:
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
}

func validateNote(note string) error {
	if n := utf8.RuneCountInString(note); n > maxNoteLen {
		return fmt.Errorf("note is %d characters long, Buzzheavier allows at most %d", n, maxNoteLen)
	}
	return nil
}

func uploadURL(o UploadOptions) string {
	u := uploadBase + "/"
	if o.ParentID != "" {
		u += url.PathEscape(o.ParentID) + "/"
	}
	u += url.PathEscape(o.Name)
	q := url.Values{}
	if o.LocationID != "" {
		q.Set("locationId", o.LocationID)
	}
	if o.Note != "" {
		q.Set("note", base64.StdEncoding.EncodeToString([]byte(o.Note)))
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// Upload sends size bytes from body. When opts.ParentID is set the request is
// authenticated and the file lands in that folder of the account; without a
// parent folder Buzzheavier treats every upload as anonymous, even when an
// Authorization header is sent.
func (c *Client) Upload(ctx context.Context, body io.Reader, size int64, opts UploadOptions) (*Item, error) {
	if err := validateName(opts.Name); err != nil {
		return nil, err
	}
	if err := validateNote(opts.Note); err != nil {
		return nil, err
	}
	if size <= 0 {
		return nil, errors.New("the file is empty, Buzzheavier does not accept empty files")
	}
	auth := opts.ParentID != ""
	if auth && c.token == "" {
		return nil, errors.New("uploading into a folder needs an Account ID")
	}
	req, err := c.newRequest(ctx, http.MethodPut, uploadURL(opts), body, auth)
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, friendlyNetErr(err)
	}
	defer resp.Body.Close()
	var it Item
	if err := decode(resp, &it); err != nil {
		return nil, err
	}
	if it.ID == "" {
		return nil, errors.New("upload finished but the server did not return a file ID")
	}
	return &it, nil
}
