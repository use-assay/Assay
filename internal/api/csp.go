package api

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	// UIScriptSHA256 is the base64-encoded SHA-256 digest of the inline <script> block in ui/index.html.
	UIScriptSHA256 = "2ju/OLdiTIubslHjB7fD3ruCghnhLZychAMkXc+P1FU="
	// UIStyleSHA256 is the base64-encoded SHA-256 digest of the inline <style> block in ui/index.html.
	UIStyleSHA256 = "nijy5j1NWnIjcLbb/zt1FoFQjF2gbKJG0DDF0k9PCAA="
)

// UICSP is the Content-Security-Policy header served with the embedded UI.
const UICSP = "default-src 'none'; script-src 'sha256-" + UIScriptSHA256 + "'; style-src 'sha256-" + UIStyleSHA256 + "'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

func init() {
	b, err := uiFS.ReadFile("ui/index.html")
	if err != nil {
		panic("api: embedded ui unavailable: " + err.Error())
	}
	scriptH, styleH, err := ExtractInlineHashes(string(b))
	if err != nil {
		panic("api: failed to extract inline hashes: " + err.Error())
	}
	if scriptH != UIScriptSHA256 || styleH != UIStyleSHA256 {
		panic(fmt.Sprintf("api: ui/index.html inline hashes do not match policy: script got %s want %s; style got %s want %s", scriptH, UIScriptSHA256, styleH, UIStyleSHA256))
	}
}

// ExtractInlineHashes extracts the single inline <style> and <script> contents from html
// and computes their base64-encoded SHA-256 digests.
func ExtractInlineHashes(html string) (scriptHash, styleHash string, err error) {
	if strings.Count(html, "<style>") != 1 || strings.Count(html, "</style>") != 1 {
		return "", "", errors.New("api: expected exactly one <style> block")
	}
	styleStart := strings.Index(html, "<style>")
	styleEnd := strings.Index(html, "</style>")
	if styleEnd < styleStart {
		return "", "", errors.New("api: malformed <style> block")
	}
	styleContent := html[styleStart+len("<style>") : styleEnd]

	if strings.Count(html, "<script>") != 1 || strings.Count(html, "</script>") != 1 {
		return "", "", errors.New("api: expected exactly one <script> block")
	}
	scriptStart := strings.Index(html, "<script>")
	scriptEnd := strings.Index(html, "</script>")
	if scriptEnd < scriptStart {
		return "", "", errors.New("api: malformed <script> block")
	}
	scriptContent := html[scriptStart+len("<script>") : scriptEnd]

	styleDigest := sha256.Sum256([]byte(styleContent))
	scriptDigest := sha256.Sum256([]byte(scriptContent))

	return base64.StdEncoding.EncodeToString(scriptDigest[:]), base64.StdEncoding.EncodeToString(styleDigest[:]), nil
}
