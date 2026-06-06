package search

import (
	"strings"
	"sistema-gestion/models"
)

// Filter es una función predicado que determina si un libro cumple
// un criterio de búsqueda. Se diseña como tipo exportado para que
// el caller pueda componer filtros arbitrarios con Apply.
type Filter func(models.Book) bool

// Searcher define el contrato de búsqueda sobre un catálogo.
// Permite cambiar la estrategia de búsqueda (en memoria, indexada, etc.)
// sin modificar a quienes la consumen.
type Searcher interface {
	Search(books []models.Book, filters ...Filter) []models.Book
}

// InMemorySearcher implementa Searcher con filtrado secuencial sobre un slice.
type InMemorySearcher struct{}

// Search aplica los filtros en conjunción lógica (AND) y retorna los libros
// que satisfacen todos los criterios simultáneamente.
func (s *InMemorySearcher) Search(books []models.Book, filters ...Filter) []models.Book {
	return Apply(books, filters...)
}

// ByAuthor retorna un Filter que busca coincidencia parcial (subcadena)
// en el campo Author, sin distinción de mayúsculas/minúsculas.
func ByAuthor(author string) Filter {
	authorLower := strings.ToLower(author)
	return func(b models.Book) bool {
		return strings.Contains(strings.ToLower(b.Author), authorLower)
	}
}

// ByTitle retorna un Filter que busca coincidencia parcial (subcadena)
// en el campo Title, sin distinción de mayúsculas/minúsculas.
func ByTitle(title string) Filter {
	titleLower := strings.ToLower(title)
	return func(b models.Book) bool {
		return strings.Contains(strings.ToLower(b.Title), titleLower)
	}
}

// ByGenre retorna un Filter que compara el género de forma exacta
// pero sin distinción de mayúsculas/minúsculas.
func ByGenre(genre string) Filter {
	return func(b models.Book) bool {
		return strings.EqualFold(b.Genre, genre)
	}
}

// ByFormat retorna un Filter que compara el formato de forma exacta.
func ByFormat(format models.BookFormat) Filter {
	return func(b models.Book) bool {
		return b.Format == format
	}
}

// ByStatus retorna un Filter que compara el estado de lectura.
func ByStatus(status models.ReadingStatus) Filter {
	return func(b models.Book) bool {
		return b.Status == status
	}
}

// Apply recibe un slice de libros y una lista de filtros, y retorna
// un nuevo slice con los libros que pasan todos los filtros (AND lógico).
// Si no se pasan filtros, retorna una copia del slice original.
//
// El algoritmo itera cada libro una sola vez y detiene la evaluación
// de filtros en el primer fallo (short-circuit), lo que lo hace
// eficiente para catálogos grandes con múltiples criterios.
func Apply(books []models.Book, filters ...Filter) []models.Book {
	if len(filters) == 0 {
		result := make([]models.Book, len(books))
		copy(result, books)
		return result
	}

	var result []models.Book
	for _, b := range books {
		match := true
		for _, f := range filters {
			if !f(b) {
				match = false
				break // short-circuit: no evaluar más filtros si ya falló uno
			}
		}
		if match {
			result = append(result, b)
		}
	}
	return result
}
