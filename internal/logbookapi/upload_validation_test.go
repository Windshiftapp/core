package logbookapi

import (
	"strings"
	"testing"
)

func TestValidateFileExtension_Allowlist(t *testing.T) {
	allowed := []string{"report.pdf", "IMG.JPG", "photo.heic", "notes.md", "slides.pptx", "data.csv"}
	for _, name := range allowed {
		if err := validateFileExtension(name); err != nil {
			t.Errorf("%q should be allowed, got error: %v", name, err)
		}
	}

	denied := []string{
		// active content / scripts
		"x.html", "x.htm", "x.xhtml", "x.svg", "x.svgz", "x.mht", "x.mhtml",
		"x.js", "x.ts", "x.mjs",
		// shell / system
		"x.sh", "x.bash", "x.ps1", "x.psm1", "x.bat", "x.cmd", "x.vbs",
		"x.hta", "x.chm", "x.reg", "x.wsf",
		// macro-bearing Office
		"x.docm", "x.xlsm", "x.pptm",
		// arbitrary XML
		"x.xml", "x.xsl", "x.xslt",
		// installers / binaries
		"x.exe", "x.msi", "x.dmg", "x.pkg", "x.apk", "x.ipa", "x.deb", "x.rpm",
		// shortcut / link
		"x.lnk", "x.url", "x.desktop",
		// extensionless
		"README", ".hidden",
	}
	for _, name := range denied {
		if err := validateFileExtension(name); err == nil {
			t.Errorf("%q should be rejected, got nil error", name)
		}
	}
}

func TestVerifyFileContent_RejectsMismatch(t *testing.T) {
	// HTML bytes claimed to be a PDF — sniffer should catch even though
	// validateFileExtension isn't called here.
	html := []byte("<!DOCTYPE html><html><body>hi</body></html>")
	if _, err := verifyFileContent(html, "x.pdf"); err == nil {
		t.Fatal("expected HTML-vs-PDF mismatch to be rejected")
	}
}

func TestVerifyFileContent_AcceptsZipBased(t *testing.T) {
	// Minimal ZIP header; DOCX, XLSX etc. all share this.
	zipHeader := []byte{0x50, 0x4b, 0x03, 0x04, 0x14, 0x00, 0x06, 0x00}
	padded := append(zipHeader, make([]byte, 512)...)
	mt, err := verifyFileContent(padded, "x.docx")
	if err != nil {
		t.Fatalf("DOCX (zip-based) should be accepted: %v", err)
	}
	if !strings.Contains(mt, "openxmlformats") && mt != "application/zip" {
		t.Fatalf("unexpected mime for docx: %q", mt)
	}
}

func TestVerifyFileContent_OctetStreamRejected(t *testing.T) {
	// Random binary bytes that sniff as application/octet-stream but have a
	// known extension — must now be rejected (previously silently accepted).
	// Note: 0xCA 0xFE 0xBA 0xBE is Mach-O / Java class magic.
	b := []byte{0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x00, 0x00, 0x34}
	if _, err := verifyFileContent(b, "report.pdf"); err == nil {
		t.Fatal("octet-stream sniff against a known extension must be rejected")
	}
}
