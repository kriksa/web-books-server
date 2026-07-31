package app
import "testing"

func TestPaginateRenderSections_ProfileModes(t *testing.T) {
	sections := []ReaderRenderSection{
		{ID: "s1", Paragraphs: []string{
			"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty",
			"alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau",
			"lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua",
		}},
	}

	pagesPaged := paginateRenderSections(sections, ReaderViewportProfile{Width: 800, Height: 600, FontSize: 20, LineHeight: 1.7, Margin: 20, Mode: "paged", Orientation: "portrait"})
	pagesDouble := paginateRenderSections(sections, ReaderViewportProfile{Width: 800, Height: 600, FontSize: 20, LineHeight: 1.7, Margin: 20, Mode: "double-page", Orientation: "portrait"})
	if len(pagesPaged) == 0 {
		t.Fatalf("expected pages for paged mode")
	}
	if len(pagesDouble) == 0 {
		t.Fatalf("expected pages for double-page mode")
	}
}

func TestExtractXhtmlText_FootnotesAndRefs(t *testing.T) {
	src := []byte(`<html><head><title>T</title></head><body><p>Hello <a epub:type="noteref" href="#n1">[1]</a> world</p><aside id="n1" class="footnote"><p>Note text</p></aside></body></html>`)
	_, pars, notes, refs := extractXhtmlText(src, "epub-sec-1")
	if len(pars) != 1 {
		t.Fatalf("expected 1 paragraph, got %d", len(pars))
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 footnote, got %d", len(notes))
	}
	if refs[0].NoteID != notes[0].ID {
		t.Fatalf("expected linked note IDs, got ref=%s note=%s", refs[0].NoteID, notes[0].ID)
	}
}

func TestFillMarkdownModel_BuildsSections(t *testing.T) {
	model := ReaderRenderModel{Title: "Book"}
	fillMarkdownModel([]byte("# Chapter 1\n\nParagraph one\n\n# Chapter 2\n\nParagraph two"), &model)
	if len(model.Sections) < 2 {
		t.Fatalf("expected >=2 sections, got %d", len(model.Sections))
	}
	if model.Sections[0].Title == "" {
		t.Fatalf("expected section title")
	}
}
