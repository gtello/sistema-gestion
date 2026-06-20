package store

import (
	"errors"
	"fmt"
	"sistema-gestion/models"
	"sync"
)

// Errores centinela exportados para que el caller pueda identificar
// la causa del fallo con errors.Is sin depender del texto del mensaje.
var (
	ErrNotFound  = errors.New("libro no encontrado")
	ErrDuplicate = errors.New("el libro ya existe en el catálogo")
)

// Repository define el contrato de persistencia en memoria para el catálogo.
// Cualquier implementación (en memoria, en base de datos, en archivo) debe
// satisfacer esta interfaz, lo que permite cambiar el almacenamiento sin
// modificar el código que lo consume.
type Repository interface {
	Add(book models.Book) error
	FindByID(id string) (models.Book, error)
	Update(id string, updateFn func(models.Book) models.Book) error
	Delete(id string) error
	ListAll() []models.Book
}

// InMemoryRepository implementa Repository sobre un slice en memoria.
// El campo books no se exporta: solo se accede a través de los métodos
// de la interfaz, garantizando encapsulación del estado interno.
type InMemoryRepository struct {
	mu    sync.RWMutex
	books []models.Book
}

// NewInMemoryRepository construye un repositorio inicializado y listo para usar.
func NewInMemoryRepository(initial []models.Book) *InMemoryRepository {
	books := make([]models.Book, len(initial))
	copy(books, initial)
	return &InMemoryRepository{books: books}
}

// Books expone el slice interno para persistencia externa (JSON).
// Es una concesión práctica: la serialización requiere acceso a los datos crudos.
// Se devuelve una copia para preservar la inmutabilidad desde fuera del paquete.
func (r *InMemoryRepository) Books() []models.Book {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]models.Book, len(r.books))
	copy(result, r.books)
	return result
}

// Add añade un libro al catálogo. Retorna ErrDuplicate si el ID ya existe.
func (r *InMemoryRepository) Add(book models.Book) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, b := range r.books {
		if b.ID == book.ID {
			return fmt.Errorf("%w: %s", ErrDuplicate, book.ID)
		}
	}
	r.books = append(r.books, book)
	return nil
}

// FindByID busca un libro por su ID. Retorna ErrNotFound si no existe.
func (r *InMemoryRepository) FindByID(id string) (models.Book, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, b := range r.books {
		if b.ID == id {
			return b, nil
		}
	}
	return models.Book{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Update aplica una función de transformación al libro identificado por id.
// updateFn es una función de orden superior que recibe el Book actual y
// devuelve una versión modificada. Este diseño permite que paquetes externos
// definan sus propias transformaciones sin que store conozca su lógica.
func (r *InMemoryRepository) Update(id string, updateFn func(models.Book) models.Book) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, b := range r.books {
		if b.ID == id {
			r.books[i] = updateFn(b)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Delete elimina un libro del catálogo por su ID.
func (r *InMemoryRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, b := range r.books {
		if b.ID == id {
			r.books = append(r.books[:i], r.books[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, id)
}

// ListAll devuelve una copia completa del catálogo.
func (r *InMemoryRepository) ListAll() []models.Book {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]models.Book, len(r.books))
	copy(result, r.books)
	return result
}
