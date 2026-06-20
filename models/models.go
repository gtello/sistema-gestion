package models

import (
	"errors"
	"fmt"
	"time"
)

// BookFormat representa los formatos de archivo soportados por el sistema.
type BookFormat string

const (
	FormatPDF  BookFormat = "PDF"
	FormatEPUB BookFormat = "EPUB"
	FormatMOBI BookFormat = "MOBI"
	FormatAZW3 BookFormat = "AZW3"
)

// ReadingStatus representa los estados del ciclo de vida de lectura.
type ReadingStatus string

const (
	StatusToRead   ReadingStatus = "POR_LEER"
	StatusReading  ReadingStatus = "LEYENDO"
	StatusFinished ReadingStatus = "LEIDO"
)

// Errores centinela para validación de datos de entrada.
// Permiten que el caller identifique el tipo de fallo sin inspeccionar cadenas.
var (
	ErrEmptyTitle  = errors.New("el título no puede estar vacío")
	ErrEmptyAuthor = errors.New("el autor no puede estar vacío")
	ErrPagesNeg    = errors.New("el número de páginas no puede ser negativo")
	ErrInvalidFmt  = errors.New("formato no válido")
)

// Book representa un libro en el catálogo.
// Los campos se mantienen exportados para que encoding/json pueda serializarlos.
type Book struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Author      string        `json:"author"`
	Genre       string        `json:"genre"`
	Format      BookFormat    `json:"format"`
	ISBN        string        `json:"isbn"`
	Pages       int           `json:"pages"`
	CurrentPage int           `json:"current_page"`
	Status      ReadingStatus `json:"status"`
	StartedAt   *time.Time    `json:"started_at,omitempty"`
	FinishedAt  *time.Time    `json:"finished_at,omitempty"`
	AddedAt     time.Time     `json:"added_at"`
}

// NewBook es el constructor canónico de Book. Centraliza la validación para
// garantizar que ningún Book inválido entre al sistema. Es la única vía de
// creación desde fuentes externas (CLI, API, etc.). La encapsulación reside en
// que el paquete caller no puede instanciar un Book con datos inconsistentes
// sin pasar por esta validación.
func NewBook(id, title, author, genre string, format BookFormat, isbn string, pages int) (Book, error) {
	if title == "" {
		return Book{}, fmt.Errorf("%w", ErrEmptyTitle)
	}
	if author == "" {
		return Book{}, fmt.Errorf("%w", ErrEmptyAuthor)
	}
	if pages < 0 {
		return Book{}, fmt.Errorf("%w: %d", ErrPagesNeg, pages)
	}
	if !isValidFormat(format) {
		return Book{}, fmt.Errorf("%w: %s", ErrInvalidFmt, format)
	}

	return Book{
		ID:      id,
		Title:   title,
		Author:  author,
		Genre:   genre,
		Format:  format,
		ISBN:    isbn,
		Pages:   pages,
		Status:  StatusToRead,
		AddedAt: time.Now(),
	}, nil
}

// isValidFormat verifica que el formato pertenezca a los valores definidos.
// No se exporta: solo NewBook debe decidir qué formatos son válidos.
func isValidFormat(f BookFormat) bool {
	switch f {
	case FormatPDF, FormatEPUB, FormatMOBI, FormatAZW3:
		return true
	default:
		return false
	}
}

// ProgressPercent calcula el avance del libro a partir de sus propias páginas.
func (b Book) ProgressPercent() float64 {
	if b.Pages == 0 {
		return 0
	}
	return float64(b.CurrentPage) / float64(b.Pages) * 100
}

// IsToRead indica si el libro aún no ha iniciado su ciclo de lectura.
func (b Book) IsToRead() bool {
	return b.Status == StatusToRead
}

// IsReading indica si el libro está actualmente en lectura.
func (b Book) IsReading() bool {
	return b.Status == StatusReading
}

// IsFinished indica si el libro ya fue marcado como leído.
func (b Book) IsFinished() bool {
	return b.Status == StatusFinished
}
