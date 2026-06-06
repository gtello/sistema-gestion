package reading

import (
	"errors"
	"fmt"
	"time"

	"sistema-gestion/models"
	"sistema-gestion/store"
)

// Errores centinela para el dominio de lectura. Cada error describe una
// violación de regla de negocio y permite al caller reaccionar en consecuencia.
var (
	ErrBookNotFound     = errors.New("libro no encontrado")
	ErrAlreadyReading   = errors.New("el libro ya está en estado LEYENDO")
	ErrAlreadyFinished  = errors.New("el libro ya fue leído")
	ErrNotReading       = errors.New("debe iniciar la lectura antes de esta operación")
	ErrInvalidPage      = errors.New("la página actual no puede ser negativa")
	ErrPageExceedsTotal = errors.New("la página actual excede el número total de páginas")
)

// ReadingService define el contrato para gestionar el ciclo de vida de lectura.
// Recibe un store.Repository como dependencia explícita en cada método,
// evitando estado implícito y facilitando pruebas unitarias con mocks.
type ReadingService interface {
	StartReading(repo store.Repository, id string, startedAt time.Time) error
	UpdateProgress(repo store.Repository, id string, currentPage int) error
	FinishReading(repo store.Repository, id string, finishedAt time.Time) error
	GetProgress(repo store.Repository, id string) (float64, error)
}

// DefaultReadingService es la implementación estándar de ReadingService.
type DefaultReadingService struct{}

// NewReadingService construye el servicio de lectura.
func NewReadingService() *DefaultReadingService {
	return &DefaultReadingService{}
}

// StartReading inicia la lectura de un libro: transición POR_LEER → LEYENDO.
// Valida que el libro exista y que no esté ya en estado LEYENDO o LEIDO.
// Registra la fecha de inicio y reinicia el contador de página actual.
func (rs *DefaultReadingService) StartReading(repo store.Repository, id string, startedAt time.Time) error {
	book, err := repo.FindByID(id)
	if err != nil {
		return fmt.Errorf("%w", ErrBookNotFound)
	}
	if book.Status == models.StatusReading {
		return fmt.Errorf("%w: %s", ErrAlreadyReading, id)
	}
	if book.Status == models.StatusFinished {
		return fmt.Errorf("%w: %s", ErrAlreadyFinished, id)
	}

	return repo.Update(id, func(b models.Book) models.Book {
		b.Status = models.StatusReading
		b.StartedAt = &startedAt
		b.CurrentPage = 0
		b.FinishedAt = nil
		return b
	})
}

// UpdateProgress actualiza la página actual de un libro en lectura.
// Solo se permite si el libro está en estado LEYENDO y la página está
// dentro del rango [0, totalPáginas].
func (rs *DefaultReadingService) UpdateProgress(repo store.Repository, id string, currentPage int) error {
	book, err := repo.FindByID(id)
	if err != nil {
		return fmt.Errorf("%w", ErrBookNotFound)
	}
	if book.Status != models.StatusReading {
		return fmt.Errorf("%w: %s", ErrNotReading, id)
	}
	if currentPage < 0 {
		return fmt.Errorf("%w: %d", ErrInvalidPage, currentPage)
	}
	if currentPage > book.Pages && book.Pages > 0 {
		return fmt.Errorf("%w: página %d de %d", ErrPageExceedsTotal, currentPage, book.Pages)
	}

	return repo.Update(id, func(b models.Book) models.Book {
		b.CurrentPage = currentPage
		return b
	})
}

// FinishReading finaliza la lectura: transición LEYENDO → LEIDO.
// Registra la fecha de finalización y ajusta la página actual al total.
// No se puede finalizar un libro que no esté en estado LEYENDO.
func (rs *DefaultReadingService) FinishReading(repo store.Repository, id string, finishedAt time.Time) error {
	book, err := repo.FindByID(id)
	if err != nil {
		return fmt.Errorf("%w", ErrBookNotFound)
	}
	if book.Status == models.StatusFinished {
		return fmt.Errorf("%w: %s", ErrAlreadyFinished, id)
	}
	if book.Status != models.StatusReading {
		return fmt.Errorf("%w: %s", ErrNotReading, id)
	}

	return repo.Update(id, func(b models.Book) models.Book {
		b.Status = models.StatusFinished
		b.FinishedAt = &finishedAt
		b.CurrentPage = b.Pages
		return b
	})
}

// GetProgress calcula el porcentaje de lectura como (páginaActual / totalPáginas * 100).
// Retorna 0 si el libro no tiene páginas registradas.
func (rs *DefaultReadingService) GetProgress(repo store.Repository, id string) (float64, error) {
	book, err := repo.FindByID(id)
	if err != nil {
		return 0, fmt.Errorf("%w", ErrBookNotFound)
	}
	if book.Pages == 0 {
		return 0, nil
	}
	return float64(book.CurrentPage) / float64(book.Pages) * 100, nil
}
