package app

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func firstFB2Author(authors []FB2Author) string {
	for _, a := range authors {
		if s := strings.TrimSpace(a.String()); s != "" {
			return s
		}
	}
	return ""
}

func extractFB2CoverData(fb2 *FB2) string {
	if fb2 == nil {
		return ""
	}
	href := strings.TrimSpace(fb2.Description.TitleInfo.Coverpage.Image.XLinkHref)
	if href == "" {
		href = strings.TrimSpace(fb2.Description.TitleInfo.Coverpage.Image.Href)
	}
	href = strings.TrimPrefix(href, "#")
	if href == "" {
		return ""
	}
	for _, b := range fb2.Binary {
		if strings.TrimSpace(b.Id) != href {
			continue
		}
		data := strings.Join(strings.Fields(b.Data), "")
		if data == "" {
			return ""
		}
		ct := strings.TrimSpace(b.ContentType)
		if ct == "" {
			ct = "image/jpeg"
		}
		return fmt.Sprintf("data:%s;base64,%s", ct, data)
	}
	return ""
}

func extractFB2CoverBytes(fb2 *FB2) ([]byte, string, error) {
	if fb2 == nil {
		return nil, "", errCoverNotFound
	}
	href := strings.TrimSpace(fb2.Description.TitleInfo.Coverpage.Image.XLinkHref)
	if href == "" {
		href = strings.TrimSpace(fb2.Description.TitleInfo.Coverpage.Image.Href)
	}
	href = strings.TrimPrefix(href, "#")
	if href == "" {
		return nil, "", errCoverNotFound
	}
	for _, b := range fb2.Binary {
		if strings.TrimSpace(b.Id) != href {
			continue
		}
		data := strings.Join(strings.Fields(b.Data), "")
		if data == "" {
			return nil, "", errCoverNotFound
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, "", err
		}
		ct := strings.TrimSpace(b.ContentType)
		if ct == "" {
			ct = "image/jpeg"
		}
		return raw, ct, nil
	}
	return nil, "", errCoverNotFound
}
