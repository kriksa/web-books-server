import re
import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[1]
mapping = [
    (r"package app\b", "package opds"),
    (r"\bBook\b", "domain.Book"),
    (r"\bOPDSFeed\b", "domain.OPDSFeed"),
    (r"\bOPDSEntry\b", "domain.OPDSEntry"),
    (r"\bOPDSLink\b", "domain.OPDSLink"),
    (r"\bOPDSAuthor\b", "domain.OPDSAuthor"),
    (r"\bOPDSText\b", "domain.OPDSText"),
    (r"\bOPDSCategory\b", "domain.OPDSCategory"),
    (r"\bOpenSearchDescription\b", "domain.OpenSearchDescription"),
    (r"\bOpenSearchURL\b", "domain.OpenSearchURL"),
    (r"\bSearchFilters\b", "domain.SearchFilters"),
    (r"\*DBManager\b", "*storage.Manager"),
    (r"\*SystemManager\b", "*Catalog"),
    (r"\bsm\.publicBaseURL\(\)", "cat.PublicBaseURL()"),
    (r"func opdsRootHandler\(sm \*Catalog\)", "func opdsRootHandler(cat *Catalog)"),
    (r"func opdsOpenSearchHandler\(sm \*Catalog\)", "func opdsOpenSearchHandler(cat *Catalog)"),
    (r"func opds(\w+)Handler\(sm \*Catalog, dm \*storage\.Manager\)", r"func opds\1Handler(cat *Catalog, dm *storage.Manager)"),
    (r"ensureFormat\(", "bookutil.EnsureFormat("),
    (r"sanitizeFilename\(", "bookutil.SanitizeFilename("),
    (r"mimeForFormat\(", "bookutil.MimeForFormat("),
]

for fn in ["opds_handlers.go", "entries.go", "grouping.go", "helpers.go", "request_url.go"]:
    text = (ROOT / "internal" / "app" / fn).read_text(encoding="utf-8")
    for pat, repl in mapping:
        text = re.sub(pat, repl, text)
    text = text.replace("for _, domain.Book := range books", "for _, book := range books")
    text = text.replace("if domain.Book.Series", "if book.Series")
    text = text.replace("append(out, domain.Book)", "append(out, book)")
    text = text.replace("func createAcquisitionEntry(domain.Book domain.Book", "func createAcquisitionEntry(book domain.Book")
    text = text.replace('safeTitle = "domain.Book"', 'safeTitle = "book"')
    text = text.replace("application/domain.OpenSearchDescription+xml", "application/opensearchdescription+xml")
    text = text.replace("func (sm *Catalog) publicBaseURL()", "")
    text = text.replace("sm.Mu.", "c.Mu.")
    text = text.replace("sm.Config", "c.Config")
    if "bookutil." in text and "web_books/internal/bookutil" not in text:
        text = text.replace(
            "package opds\nimport (",
            "package opds\nimport (\n\t\"web_books/internal/bookutil\"\n\t\"web_books/internal/domain\"\n\t\"web_books/internal/storage\"\n",
            1,
        )
    out = "url.go" if fn == "request_url.go" else fn
    (ROOT / "internal" / "opds" / out).write_text(text, encoding="utf-8")
print("ok")
