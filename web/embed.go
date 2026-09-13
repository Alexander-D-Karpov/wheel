// Package web carries the embedded browser assets, so a deployed build is a
// single binary with no files to copy alongside it.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io"
	"io/fs"
)

//go:embed static
var staticRoot embed.FS

// Templates holds the HTML page templates.
//
//go:embed templates/*.html
var Templates embed.FS

// Static serves the files under /static/, rooted so that "app.css" resolves.
var Static = mustSub(staticRoot, "static")

// AssetVersion is a short digest of every static file. It is appended to asset
// URLs so browsers can cache them forever yet still pick up a new deploy.
var AssetVersion = digestStatic()

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic("web: embedded assets are missing: " + err.Error())
	}
	return sub
}

func digestStatic() string {
	h := sha256.New()
	err := fs.WalkDir(Static, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := Static.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		io.WriteString(h, path)
		_, err = io.Copy(h, f)
		return err
	})
	if err != nil {
		return "dev"
	}
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))[:10]
}
