package fitz

import (
	"fmt"
	"image/png"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	doc, err := New(filepath.Join("testdata", "Profoto.pdf"))
	if err != nil {
		t.Error(err)
	}
	fmt.Println(doc.NumObj(), doc.NumPage())
}

func TestNewFromMemory(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("testdata", "Profoto.pdf"))
	if err != nil {
		t.Error(err)
	}

	doc, err := NewFromMemory(b)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(doc.NumObj(), doc.NumPage())
}

func TestNewFromReader(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "Profoto.pdf"))
	if err != nil {
		t.Error(err)
	}

	doc, err := NewFromReader(f)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(doc.NumObj(), doc.NumPage())
}

func TestText(t *testing.T) {
	doc, err := New(filepath.Join("testdata", "liusi.pdf"))
	if err != nil {
		t.Error(err)
	}

	defer doc.Close()

	//tmpDir, err := ioutil.TempDir(os.TempDir(), "fitz")
	//if err != nil {
	//	t.Error(err)
	//}

	startTime := time.Now()

	var sb strings.Builder
	random := rand.New(rand.NewSource(startTime.Unix()))

	for n := 0; n < doc.NumPage(); n++ {
		text, err := doc.Text(n)
		if err != nil {
			t.Error(err)
		}
		if sb.Len() > 100000 {
			break
		}
		if sb.Len() < 50000 {
			sb.WriteString(text)
			continue
		}

		if random.Float32() > 0.5 {
			sb.WriteString(text)
		}

		//f, err := os.Create(filepath.Join(tmpDir, fmt.Sprintf("test%03d.txt", n)))
		//if err != nil {
		//	t.Error(err)
		//}
		//
		//_, err = f.WriteString(text)
		//if err != nil {
		//	t.Error(err)
		//}
		//
		//f.Close()
	}
	fmt.Println("len:", len(sb.String()), ",use:", time.Now().Sub(startTime))
}

func TestImage(t *testing.T) {
	doc, err := New(filepath.Join("testdata", "Profoto.pdf"))
	if err != nil {
		t.Error(err)
	}

	defer doc.Close()

	for n := 1; n < doc.NumObj(); n++ {
		img, err := doc.Image(n)
		if err != nil {
			if err != ErrNotImage {
				t.Logf("Get Image failed, err=%v", err)
			}
			continue
		}

		f, err := os.Create(filepath.Join("test", fmt.Sprintf("img-%v.png", n)))
		if err != nil {
			t.Error(err)
		}

		err = png.Encode(f, img)
		if err != nil {
			t.Error(err)
		}

		f.Close()
	}
}

func TestToC(t *testing.T) {
	doc, err := New(filepath.Join("testdata", "test.pdf"))
	if err != nil {
		t.Error(err)
	}

	defer doc.Close()

	outlines, err := doc.ToC()
	if err != nil {
		t.Error(err)
	}
	fmt.Println(outlines)
}

func TestMetadata(t *testing.T) {
	doc, err := New(filepath.Join("testdata", "test.pdf"))
	if err != nil {
		t.Error(err)
	}

	defer doc.Close()

	meta := doc.Metadata()
	if len(meta) == 0 {
		t.Error(fmt.Errorf("metadata is empty"))
	}
	fmt.Println(meta)
}
