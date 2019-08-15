// Package fitz provides wrapper for the [MuPDF](http://mupdf.com/) fitz library
// that can extract pages from PDF and EPUB documents as images, text, html or svg.
package fitz

/*
#include <mupdf/fitz.h>
#include <mupdf/pdf.h>
#include <stdlib.h>

const char *fz_version = FZ_VERSION;
*/
import "C"

import (
	"bytes"
	"errors"
	"image"
	_ "image/png"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

// Errors.
var (
	ErrNoSuchFile    = errors.New("fitz: no such file")
	ErrCreateContext = errors.New("fitz: cannot create context")
	ErrOpenDocument  = errors.New("fitz: cannot open document")
	ErrOpenMemory    = errors.New("fitz: cannot open memory")
	ErrPageMissing   = errors.New("fitz: page missing")
	ErrObjMissing    = errors.New("fitz: obj missing")
	ErrNotImage      = errors.New("fitz: obj is not image, please ignore this obj")
	ErrCreatePixmap  = errors.New("fitz: cannot create pixmap")
	ErrPixmapSamples = errors.New("fitz: cannot get pixmap samples")
	ErrNeedsPassword = errors.New("fitz: document needs password")
	ErrLoadOutline   = errors.New("fitz: cannot load outline")
)

// Document represents fitz document.
type Document struct {
	ctx       *C.struct_fz_context_s
	pdf       *C.struct_pdf_document_s
	pageTotal int
	objTotal  int
	mtx       sync.Mutex //todo:delete lock to be more fast
}

// Outline type.
type Outline struct {
	// Hierarchy level of the entry (starting from 1).
	Level int
	// Title of outline item.
	Title string
	// Destination in the document to be displayed when this outline item is activated.
	URI string
	// The page number of an internal link.
	Page int
	// Top.
	Top float64
}

// New returns new fitz document from filename. After process, please do Close() to release resource.
func New(filename string) (f *Document, err error) {
	f = &Document{}
	filename, err = filepath.Abs(filename)
	if err != nil {
		return
	}

	if _, e := os.Stat(filename); e != nil {
		err = ErrNoSuchFile
		return
	}

	f.ctx = (*C.struct_fz_context_s)(unsafe.Pointer(C.fz_new_context_imp(nil, nil, C.FZ_STORE_UNLIMITED, C.fz_version)))
	if f.ctx == nil {
		err = ErrCreateContext
		return
	}

	C.fz_register_document_handlers(f.ctx)

	cfilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cfilename))

	f.pdf = C.pdf_open_document(f.ctx, cfilename)
	if f.pdf == nil {
		err = ErrOpenDocument
	}
	ret := C.pdf_needs_password(f.ctx, f.pdf)
	v := bool(int(ret) != 0)
	if v {
		err = ErrNeedsPassword
	}

	f.pageTotal = int(C.pdf_count_pages(f.ctx, f.pdf))
	f.objTotal = int(C.pdf_count_objects(f.ctx, f.pdf))

	return
}

// NewFromMemory returns new fitz document from byte slice, please do Close() to release resource.
func NewFromMemory(b []byte) (f *Document, err error) {
	f = &Document{}

	f.ctx = (*C.struct_fz_context_s)(unsafe.Pointer(C.fz_new_context_imp(nil, nil, C.FZ_STORE_UNLIMITED, C.fz_version)))
	if f.ctx == nil {
		err = ErrCreateContext
		return
	}

	C.fz_register_document_handlers(f.ctx)

	data := (*C.uchar)(C.CBytes(b))

	stream := C.fz_open_memory(f.ctx, data, C.size_t(len(b)))
	if stream == nil {
		err = ErrOpenMemory
		return
	}
	defer C.fz_drop_stream(f.ctx, stream)

	f.pdf = C.pdf_open_document_with_stream(f.ctx, stream)
	if f.pdf == nil {
		err = ErrOpenDocument
	}

	ret := C.pdf_needs_password(f.ctx, f.pdf)
	v := bool(int(ret) != 0)
	if v {
		err = ErrNeedsPassword
	}

	f.pageTotal = int(C.pdf_count_pages(f.ctx, f.pdf))
	f.objTotal = int(C.pdf_count_objects(f.ctx, f.pdf))

	return
}

// NewFromReader returns new fitz document from io.Reader, please do Close() to release resource.
func NewFromReader(r io.Reader) (f *Document, err error) {
	b, e := ioutil.ReadAll(r)
	if e != nil {
		err = e
		return
	}

	return NewFromMemory(b)
}

// NumPage returns total number of pages in document.
func (f *Document) NumPage() int {
	return f.pageTotal
}

// NumObj returns total number of objects in document.
func (f *Document) NumObj() int {
	return f.objTotal
}

// Text returns text for given page number.  Index start at 0
func (f *Document) Text(pageNumber int) (string, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if pageNumber < 0 || f.pageTotal <= pageNumber {
		return "", ErrPageMissing
	}

	page := C.pdf_load_page(f.ctx, f.pdf, C.int(pageNumber))
	defer C.fz_drop_page(f.ctx, (*C.fz_page)(unsafe.Pointer(page)))

	var bounds C.fz_rect
	C.pdf_bound_page(f.ctx, page, &bounds)

	var ctm C.fz_matrix
	C.fz_scale(&ctm, C.float(1.0), C.float(1.0))

	text := C.fz_new_stext_page(f.ctx, &bounds)
	defer C.fz_drop_stext_page(f.ctx, text)

	var opts C.fz_stext_options
	opts.flags = 0

	device := C.fz_new_stext_device(f.ctx, text, &opts)
	C.fz_enable_device_hints(f.ctx, device, C.FZ_NO_CACHE)
	defer C.fz_drop_device(f.ctx, device)

	var cookie C.fz_cookie
	C.pdf_run_page(f.ctx, page, device, &ctm, &cookie)

	C.fz_close_device(f.ctx, device)

	buf := C.fz_new_buffer_from_stext_page(f.ctx, text)
	defer C.fz_drop_buffer(f.ctx, buf)

	str := C.GoString(C.fz_string_from_buffer(f.ctx, buf))

	return str, nil
}

// Image returns image.Image encoded by png. The objNumber should between 1 ~ f.NumObj()
func (f *Document) Image(objNumber int) (image.Image, error) {
	b, err := f.ImageBytes(objNumber)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewBuffer(b))
	return img, err
}

// ImageBytes returns image.Image bytes encoded by png. The objNumber should between 1 ~ f.NumObj()
// ImageBytes will be faster than Image
func (f *Document) ImageBytes(objNumber int) ([]byte, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if objNumber <= 0 || f.objTotal <= objNumber {
		return nil, ErrObjMissing
	}

	obj := C.pdf_load_object(f.ctx, f.pdf, C.int(objNumber))
	if f.isImage(obj) {
		return f.saveImage(objNumber), nil
	}

	return nil, ErrNotImage
}

func (f *Document) isImage(obj *C.pdf_obj) bool {
	objType := C.pdf_dict_get(f.ctx, obj, C.PDF_NAME_Subtype);
	return C.int(1) == C.pdf_name_eq(f.ctx, objType, C.PDF_NAME_Image)
}

func (f *Document) saveImage(objNumber int) []byte {
	ref := C.pdf_new_indirect(f.ctx, f.pdf, C.int(objNumber), C.int(0))
	defer C.pdf_drop_obj(f.ctx, ref)

	fzImg := C.pdf_load_image(f.ctx, f.pdf, ref)
	defer C.fz_drop_image(f.ctx, fzImg)

	buf := C.fz_new_buffer_from_image_as_png(f.ctx, fzImg, nil)
	defer C.fz_drop_buffer(f.ctx, buf)

	size := C.fz_buffer_storage(f.ctx, buf, nil)
	str := C.GoStringN(C.fz_string_from_buffer(f.ctx, buf), C.int(size))

	return []byte(str)
}

// ToC returns the table of contents (also known as outline).
func (f *Document) ToC() ([]Outline, error) {
	data := make([]Outline, 0)

	outline := C.pdf_load_outline(f.ctx, f.pdf)
	if outline == nil {
		return nil, ErrLoadOutline
	}
	defer C.fz_drop_outline(f.ctx, outline)

	var walk func(outline *C.fz_outline, level int)

	walk = func(outline *C.fz_outline, level int) {
		for outline != nil {
			res := Outline{}
			res.Level = level
			res.Title = C.GoString(outline.title)
			res.URI = C.GoString(outline.uri)
			res.Page = int(outline.page)
			res.Top = float64(outline.y)
			data = append(data, res)

			if outline.down != nil {
				walk(outline.down, level+1)
			}
			outline = outline.next
		}
	}

	walk(outline, 1)
	return data, nil
}

// Metadata returns the map with standard metadata.
func (f *Document) Metadata() map[string]string {
	data := make(map[string]string)

	lookup := func(key string) string {
		ckey := C.CString(key)
		defer C.free(unsafe.Pointer(ckey))

		buf := make([]byte, 256)
		C.pdf_lookup_metadata(f.ctx, f.pdf, ckey, (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))

		buf = bytes.TrimRight(buf, "\x00")
		return string(buf)
	}

	data["format"] = lookup("format")
	data["encryption"] = lookup("encryption")
	data["title"] = lookup("info:Title")
	data["author"] = lookup("info:Author")
	data["subject"] = lookup("info:Subject")
	data["keywords"] = lookup("info:Keywords")
	data["creator"] = lookup("info:Creator")
	data["producer"] = lookup("info:Producer")
	data["creationDate"] = lookup("info:CreationDate")
	data["modDate"] = lookup("info:modDate")

	return data
}

// Close closes the underlying fitz document.
func (f *Document) Close() error {
	C.pdf_drop_document(f.ctx, f.pdf)
	C.fz_drop_context(f.ctx)
	return nil
}

// contentType returns document MIME type.
func contentType(b []byte) string {
	var mtype string
	if len(b) > 3 && b[0] == 0x25 && b[1] == 0x50 && b[2] == 0x44 && b[3] == 0x46 {
		mtype = "application/pdf"
	} else if len(b) > 57 && b[0] == 0x50 && b[1] == 0x4B && b[2] == 0x3 && b[3] == 0x4 && b[30] == 0x6D && b[31] == 0x69 && b[32] == 0x6D && b[33] == 0x65 &&
		b[34] == 0x74 && b[35] == 0x79 && b[36] == 0x70 && b[37] == 0x65 && b[38] == 0x61 && b[39] == 0x70 && b[40] == 0x70 && b[41] == 0x6C &&
		b[42] == 0x69 && b[43] == 0x63 && b[44] == 0x61 && b[45] == 0x74 && b[46] == 0x69 && b[47] == 0x6F && b[48] == 0x6E && b[49] == 0x2F &&
		b[50] == 0x65 && b[51] == 0x70 && b[52] == 0x75 && b[53] == 0x62 && b[54] == 0x2B && b[55] == 0x7A && b[56] == 0x69 && b[57] == 0x70 {
		mtype = "application/epub+zip"
	}
	return mtype
}
