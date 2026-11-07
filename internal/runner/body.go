package runner

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/environments"
)

// builtBody is a request body ready to send. Summary describes it for
// history and scripts: files are listed by name and size, never inlined.
type builtBody struct {
	data        []byte
	contentType string
	summary     string
}

// buildBody resolves variables in req's body and encodes it for its body type.
// Files are read here, so a missing or oversized file is reported before sending.
func buildBody(req domain.Request, res *environments.Resolver) (*builtBody, error) {
	switch req.BodyType {
	case domain.BodyJSON:
		b := res.Resolve(req.Body)
		return &builtBody{data: []byte(b), contentType: "application/json", summary: b}, nil
	case domain.BodyText:
		b := res.Resolve(req.Body)
		return &builtBody{data: []byte(b), contentType: "text/plain; charset=utf-8", summary: b}, nil
	case domain.BodyURLEncoded:
		var parts []string
		for _, f := range req.Form {
			if !f.Enabled || strings.TrimSpace(f.Key) == "" {
				continue
			}
			if f.Type == domain.FormFile {
				return nil, fmt.Errorf("form field %q is a file; URL-encoded bodies can only send text (use form-data)", f.Key)
			}
			parts = append(parts, url.QueryEscape(res.Resolve(f.Key))+"="+url.QueryEscape(res.Resolve(f.Value)))
		}
		b := strings.Join(parts, "&")
		return &builtBody{data: []byte(b), contentType: "application/x-www-form-urlencoded", summary: b}, nil
	case domain.BodyFormData:
		return buildMultipart(req.Form, res)
	case domain.BodyBinary:
		f, err := readUpload(req.BodyFile, res, 0, "body")
		if err != nil {
			return nil, err
		}
		return &builtBody{data: f.data, contentType: f.contentType, summary: f.describe()}, nil
	}
	return nil, nil
}

func buildMultipart(fields []domain.FormField, res *environments.Resolver) (*builtBody, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	var summary []string
	var total int64
	for _, f := range fields {
		if !f.Enabled || strings.TrimSpace(f.Key) == "" {
			continue
		}
		name := res.Resolve(f.Key)
		if f.Type != domain.FormFile {
			value := res.Resolve(f.Value)
			if err := w.WriteField(name, value); err != nil {
				return nil, err
			}
			summary = append(summary, name+"="+value)
			continue
		}
		up, err := readUpload(f.File, res, total, fmt.Sprintf("form field %q", name))
		if err != nil {
			return nil, err
		}
		total += int64(len(up.data))
		ct := up.contentType
		if c := strings.TrimSpace(res.Resolve(f.ContentType)); c != "" {
			if strings.ContainsAny(c, "\r\n") {
				return nil, fmt.Errorf("form field %q: content type contains a line break after variable substitution", name)
			}
			ct = c
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": name, "filename": up.name}))
		h.Set("Content-Type", ct)
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(up.data); err != nil {
			return nil, err
		}
		up.contentType = ct
		summary = append(summary, name+"="+up.describe())
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return &builtBody{data: buf.Bytes(), contentType: w.FormDataContentType(), summary: strings.Join(summary, "\n")}, nil
}

type upload struct {
	name, contentType string
	data              []byte
}

func (u upload) describe() string {
	return fmt.Sprintf("@%s (%s, %s)", u.name, formatBytes(int64(len(u.data))), u.contentType)
}

// readUpload reads the file at the resolved path. used is what the request
// already uploads, so the total stays within domain.MaxUploadBytes.
func readUpload(pathTemplate string, res *environments.Resolver, used int64, what string) (*upload, error) {
	path := strings.TrimSpace(res.Resolve(pathTemplate))
	if path == "" {
		return nil, fmt.Errorf("%s: choose a file to upload", what)
	}
	if strings.Contains(path, "{{") {
		// The resolver reports undefined variables; do not try to open the template.
		return &upload{name: filepath.Base(path), contentType: "application/octet-stream"}, nil
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%s: file path %q must be absolute", what, path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot open the file: %w", what, unwrapPath(err))
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, unwrapPath(err))
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %s is not a regular file", what, path)
	}
	if used+st.Size() > domain.MaxUploadBytes {
		return nil, fmt.Errorf("%s: files larger than %d MB in total cannot be uploaded", what, domain.MaxUploadBytes>>20)
	}
	data, err := io.ReadAll(io.LimitReader(f, domain.MaxUploadBytes-used+1))
	if err != nil {
		return nil, fmt.Errorf("%s: cannot read the file: %w", what, err)
	}
	if used+int64(len(data)) > domain.MaxUploadBytes {
		return nil, fmt.Errorf("%s: files larger than %d MB in total cannot be uploaded", what, domain.MaxUploadBytes>>20)
	}
	ct := mime.TypeByExtension(filepath.Ext(path))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	return &upload{name: filepath.Base(path), contentType: ct, data: data}, nil
}

func unwrapPath(err error) error {
	if pe, ok := err.(*os.PathError); ok {
		return pe.Err
	}
	return err
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
