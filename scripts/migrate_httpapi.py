import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[1]
text = (ROOT / "internal/app/handlers_api.go").read_text(encoding="utf-8")
text = text.replace("package app", "package httpapi", 1)
text = re.sub(r"\*DBManager\b", "*storage.Manager", text)
text = re.sub(r"\*SystemManager\b", "*Host", text)
text = re.sub(r"func apiSearchHandler\(sm \*Host, dm \*storage\.Manager\)", "func apiSearchHandler(_ *Host, dm *storage.Manager)", text)
text = text.replace("extractBookBytes(dm, booksDir, bookID)", "dm.ExtractBookBytes(booksDir, bookID)")
text = text.replace("LoadConfig()", "config.Load()")
text = text.replace("SaveConfig(cfg)", "config.Save(cfg)")
text = text.replace("domain.CoverExtractTimeout", "domain.CoverExtractTimeout")  # noop
text = text.replace("errCoverNotFound", "domain.ErrCoverNotFound")
text = text.replace("SearchFilters{", "domain.SearchFilters{")
text = text.replace("DetailedBookInfo", "domain.DetailedBookInfo")
text = text.replace("ConfigRequest", "domain.ConfigRequest")
text = text.replace("ParseFB2Metadata", "parseFB2")  # will add helper
text = text.replace("ConvertPamphletToDetails", "convertPamphlet")
text = text.replace("ExtractDetailedInfo", "extractDetailed")
text = text.replace("findZipEntry", "findZip")  # local in httpapi cover

imports = '''package httpapi

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/timsims/pamphlet"
	"golang.org/x/crypto/bcrypt"
	lru "github.com/hashicorp/golang-lru/v2"

	"web_books/internal/bookutil"
	"web_books/internal/config"
	"web_books/internal/domain"
	"web_books/internal/formats/epub"
	"web_books/internal/storage"
)

'''
# strip old imports from text
body = text.split("import (", 1)[1]
body = body.split(")\n", 1)[1]
# remove duplicate const webAuthCookieName block start - keep from first func
(ROOT / "internal/httpapi/handlers.go").write_text(imports + body.lstrip(), encoding="utf-8")
print("ok")
