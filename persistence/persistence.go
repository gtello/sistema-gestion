package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sistema-gestion/models"
)

// Errores centinela para operaciones de persistencia.
var (
	ErrLoad = errors.New("error al cargar los datos")
	ErrSave = errors.New("error al guardar los datos")
)

// Storage define el contrato de persistencia para el catálogo de libros.
// Desacopla el almacenamiento físico (archivo, base de datos, memoria)
// del resto del sistema. Cualquier implementación que satisfaga esta
// interfaz puede usarse sin modificar la CLI ni los servicios.
type Storage interface {
	Load() ([]models.Book, error)
	Save(books []models.Book) error
}

// JSONStorage implementa Storage usando un archivo JSON local.
// El campo path no se exporta: una vez construido, el consumidor
// no puede cambiar la ruta del archivo, protegiendo la integridad
// de los datos.
type JSONStorage struct {
	path string
}

// NewJSONStorage construye un almacenamiento JSON. La ruta es inmutable
// durante el ciclo de vida del programa.
func NewJSONStorage(path string) *JSONStorage {
	return &JSONStorage{path: path}
}

// Load lee el archivo JSON y decodifica el catálogo. Retorna slice vacío
// si el archivo no existe (primera ejecución), y envuelve el error con
// ErrLoad para otros fallos.
func (s *JSONStorage) Load() ([]models.Book, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.Book{}, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrLoad, err)
	}

	var books []models.Book
	if err := json.Unmarshal(data, &books); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLoad, err)
	}
	return books, nil
}

// Save serializa el catálogo a JSON indentado y lo escribe en disco.
// Envuelve el error con ErrSave para que el caller pueda identificarlo.
func (s *JSONStorage) Save(books []models.Book) error {
	data, err := json.MarshalIndent(books, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSave, err)
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("%w: %v", ErrSave, err)
	}
	return nil
}
