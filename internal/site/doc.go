// Package site renders a data repository as a static website, an Atom feed
// and the plain text files that finger would have shown.
//
// Links between pages are relative, so the site works under any base path.
// The 404 page is the exception: GitHub Pages serves it at whatever URL was
// missed, so it links from the root instead.
package site
