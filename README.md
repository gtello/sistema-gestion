# Sistema de Gestión de E-Books

Sistema de línea de comandos para administrar una biblioteca personal de libros digitales. Desarrollado en Go bajo el paradigma de **programación funcional**: cero métodos en structs, inmutabilidad de datos y funciones de orden superior como mecanismo principal de composición.

## Alcance del sistema

- Registrar libros con metadatos completos: título, autor, género, formato, ISBN y número de páginas.
- Consultar el catálogo completo o ver el detalle de un libro específico.
- Buscar libros combinando múltiples criterios de filtrado.
- Gestionar el ciclo de vida de lectura: por leer → leyendo → leído, con seguimiento de progreso por página.
- Persistir los datos en archivos JSON para que sobrevivan entre ejecuciones.

**Fuera de alcance:** edición del contenido del libro, renderizado de formatos, sincronización con dispositivos externos, autenticación de usuarios.

## Estructura del proyecto

```
sistema-gestion/
├── main.go
├── models/
│   └── models.go
├── store/
│   └── store.go
├── search/
│   └── search.go
├── reading/
│   └── reading.go
├── persistence/
│   └── persistence.go
├── cli/
│   └── cli.go
└── web/
    ├── server.go
    └── templates/
        ├── layout.html
        ├── list.html
        ├── add.html
        └── info.html
```

## Módulos

### `models`: Tipos de datos

Define las estructuras de datos puras que recorren todo el sistema. Ningún struct contiene métodos; son exclusivamente contenedores de información.

| Tipo | Descripción |
|------|-------------|
| `Book` | Estructura central: título, autor, género, formato, ISBN, páginas, página actual, estado de lectura y fechas de inicio/fin/alta. |
| `BookFormat` | Tipo enumerado: `PDF`, `EPUB`, `MOBI`, `AZW3`. |
| `ReadingStatus` | Tipo enumerado: `POR_LEER`, `LEYENDO`, `LEIDO`. |

**Responsabilidad única:** modelar el dominio sin acoplar lógica de negocio a los datos.

### `store`: CRUD funcional

Implementa las operaciones de almacenamiento en memoria sobre un slice de `Book`. Cada función recibe el estado actual y devuelve un estado nuevo, sin mutar el original.

| Función | Firma | Descripción |
|---------|-------|-------------|
| `Add` | `([]Book, Book) → ([]Book, error)` | Añade un libro nuevo. Rechaza IDs duplicados. |
| `FindByID` | `([]Book, string) → (Book, bool)` | Busca un libro por ID. Retorna un booleano de encontrado en lugar de `nil`. |
| `Update` | `([]Book, string, func(Book) Book) → ([]Book, error)` | Aplica una función de transformación al libro indicado. La función `updateFn` es el mecanismo de extensión: cualquier cambio futuro se modela como una función pasada a `Update`, sin modificar el paquete. |
| `Delete` | `([]Book, string) → ([]Book, error)` | Elimina un libro por ID. |
| `ListAll` | `([]Book) → []Book` | Devuelve una copia del catálogo completo. |

**Patrón funcional clave:** `Update` recibe una función de orden superior `func(Book) Book`. Esto permite que paquetes externos (`reading`) definan transformaciones sin que `store` conozca sus reglas de negocio.

### `search`: Filtros componibles

Define `Filter` como `func(Book) bool` y provee constructores de filtros que se combinan con `Apply`.

| Función | Firma | Descripción |
|---------|-------|-------------|
| `ByAuthor` | `(string) → Filter` | Filtro por autor (búsqueda parcial, case-insensitive). |
| `ByTitle` | `(string) → Filter` | Filtro por título (búsqueda parcial, case-insensitive). |
| `ByGenre` | `(string) → Filter` | Filtro exacto por género (case-insensitive). |
| `ByFormat` | `(BookFormat) → Filter` | Filtro exacto por formato. |
| `ByStatus` | `(ReadingStatus) → Filter` | Filtro exacto por estado de lectura. |
| `Apply` | `([]Book, ...Filter) → []Book` | Aplica múltiples filtros en conjunción (AND lógico). |

**Patrón funcional clave:** cada filtro es una función pura que se compone sin efectos secundarios. Agregar un nuevo criterio de búsqueda no requiere modificar ninguna función existente, solo se añade un nuevo constructor de `Filter`.

### `reading`: Ciclo de lectura

Gestiona las transiciones de estado del proceso de lectura usando las funciones de `store` como base.

| Función | Firma | Descripción |
|---------|-------|-------------|
| `StartReading` | `([]Book, string, time.Time) → ([]Book, error)` | Transición `POR_LEER → LEYENDO`. Registra la fecha de inicio. Valida que el libro no esté ya leído o en lectura. |
| `UpdateProgress` | `([]Book, string, int) → ([]Book, error)` | Actualiza la página actual. Solo permitido en estado `LEYENDO`. Valida que la página esté dentro del rango. |
| `FinishReading` | `([]Book, string, time.Time) → ([]Book, error)` | Transición `LEYENDO → LEIDO`. Registra la fecha de finalización y ajusta la página actual al total de páginas. |
| `GetProgress` | `([]Book, string) → (float64, error)` | Calcula el porcentaje de avance (`currentPage / pages * 100`). |

**Patrón funcional clave:** ninguna función de este paquete accede directamente al slice. Todas las mutaciones se delegan a `store.Update`, que recibe una función de transformación pura. La lógica de validación (`StartReading`, `UpdateProgress`, `FinishReading`) está separada de la lógica de actualización del slice.

### `persistence`: Serialización JSON

Capa de entrada/salida que convierte entre el slice en memoria y el archivo en disco.

| Función | Firma | Descripción |
|---------|-------|-------------|
| `LoadBooks` | `(string) → ([]Book, error)` | Lee y decodifica un archivo JSON. Retorna slice vacío si el archivo no existe. |
| `SaveBooks` | `(string, []Book) → error` | Codifica y escribe el slice a un archivo JSON con indentación. |

### `cli`: Interfaz de línea de comandos

Orquesta todos los módulos anteriores. Usa el paquete `flag` de la biblioteca estándar para procesar subcomandos y opciones.

| Comando | Opciones | Descripción |
|---------|----------|-------------|
| `add` | `-title`, `-author`, `-genre`, `-format`, `-isbn`, `-pages` | Añade un libro al catálogo. |
| `list` | — | Muestra todos los libros en formato tabla. |
| `search` | `-author`, `-title`, `-genre`, `-format`, `-status` | Busca libros combinando filtros (AND). |
| `info` | `-id` | Muestra el detalle completo de un libro, incluyendo progreso. |
| `delete` | `-id` | Elimina un libro del catálogo. |
| `reading start` | `-id` | Inicia la lectura de un libro. |
| `reading progress` | `-id`, `-page` | Actualiza la página actual. |
| `reading finish` | `-id` | Marca un libro como leído. |

**Patrón funcional clave:** la función `RunWithDeps` recibe las dependencias como parámetros explícitos (inyección de dependencias manual), permitiendo que `main.go` construya las dependencias una sola vez y las comparta con el modo web.

### `web`: Interfaz gráfica web

Servidor HTTP que expone el sistema mediante páginas HTML renderizadas en el servidor. Usa `net/http` y `html/template` sin dependencias externas ni JavaScript. Las plantillas se empaquetan en el binario con `embed`.

| Ruta | Método | Descripción |
|------|--------|-------------|
| `/` | GET | Catálogo principal con búsqueda integrada y botones de acción. |
| `/books/add` | GET/POST | Formulario de alta de libro. GET muestra el formulario; POST valida y guarda. |
| `/books/info?id=` | GET | Ficha detallada del libro con progreso de lectura. |
| `/books/delete` | POST | Elimina un libro del catálogo. |
| `/books/reading/start` | POST | Inicia la lectura de un libro. |
| `/books/reading/progress` | POST | Actualiza la página actual desde el formulario en línea. |
| `/books/reading/finish` | POST | Marca el libro como leído. |

**Patrón funcional clave:** el servidor recibe las mismas interfaces que la CLI (`Repository`, `Storage`, `ReadingService`, `Searcher`). Los handlers son closures que capturan estas dependencias en lugar de usar variables globales. La lógica de negocio no se duplica: web y CLI comparten el mismo núcleo funcional.

### Interfaces

El sistema define contratos explícitos que desacoplan la lógica de negocio de las implementaciones concretas:

| Interfaz | Paquete | Métodos | Implementación actual |
|----------|---------|---------|----------------------|
| `Repository` | `store` | `Add`, `FindByID`, `Update`, `Delete`, `ListAll` | `InMemoryRepository` (slice en memoria) |
| `Storage` | `persistence` | `Load`, `Save` | `JSONStorage` (archivo JSON en disco) |
| `ReadingService` | `reading` | `StartReading`, `UpdateProgress`, `FinishReading`, `GetProgress` | `DefaultReadingService` (validación + delegación a Repository) |
| `Searcher` | `search` | `Search([]Book, ...Filter) []Book` | `InMemorySearcher` (filtrado secuencial) |

Cada interfaz permite cambiar la implementación sin modificar el código que la consume. Por ejemplo, `InMemoryRepository` podría reemplazarse por `SQLiteRepository` sin tocar la CLI, la web ni los servicios.

### Encapsulación

| Paquete | Qué se encapsula | Mecanismo |
|---------|-----------------|-----------|
| `models` | Validación de datos de entrada | `NewBook` como único constructor público; `isValidFormat` no exportada |
| `store` | Estado interno del repositorio | `InMemoryRepository.books` no exportado; `ListAll()` devuelve copia |
| `persistence` | Ruta del archivo de datos | `JSONStorage.path` no exportado, asignado solo en `NewJSONStorage` |
| `web` | Dependencias del servidor | Campos de `Server` no exportados; solo se accede vía handlers |
| `reading` | Reglas de transición de estados | Validaciones internas antes de delegar a `Repository.Update` |

---

## Paquetes incorporados usados

| Paquete | Uso | Justificación |
|---------|-----|---------------|
| `encoding/json` | `persistence` | `MarshalIndent` y `Unmarshal` para serializar/deserializar el catálogo a JSON legible. Alternativas como `gob` o `protobuf` son binarias y no permiten inspeccionar el archivo manualmente. |
| `os` | `persistence`, `cli` | `ReadFile` y `WriteFile` para E/S de archivos; `os.Args` para leer argumentos de línea de comandos; `os.Stderr` para mensajes de error. La biblioteca estándar cubre completamente estas necesidades sin dependencias externas. |
| `flag` | `cli` | Procesado de subcomandos y banderas con `FlagSet`. Elegido sobre `os.Args` manual porque maneja automáticamente la validación de tipos, valores por defecto y mensajes de ayuda. No se usó `cobra` ni `urfave/cli` para mantener cero dependencias externas, alineado con el principio de simplicidad del proyecto. |
| `time` | `models`, `reading`, `cli` | Registro de fechas de alta, inicio de lectura y finalización. `time.Time` ofrece comparación, formateo y serialización JSON sin lógica adicional. |
| `errors` | `store`, `reading` | Creación de errores descriptivos con `errors.New`. Retorna `error` como segundo valor de retorno (convención Go) en lugar de usar `panic`, que detendría la ejecución. |
| `strings` | `search` | `Contains` y `ToLower` para búsquedas parciales case-insensitive por autor y título. |
| `fmt` | `cli`, `web` | Salida formateada a consola y formateo de strings para mensajes flash. Usado exclusivamente en la capa de presentación. |
| `net/http` | `web` | Servidor HTTP y enrutamiento con `ServeMux`. Elegido sobre frameworks externos (Gin, Echo, Chi) por ser parte de la biblioteca estándar y cubrir completamente las necesidades de una SPA renderizada en servidor sin APIs REST complejas. |
| `html/template` | `web` | Renderizado de plantillas HTML con escape automático de XSS. Las funciones de plantilla (`statusBadge`, `percent`) se registran como `FuncMap` para mantener la lógica de presentación en Go y las plantillas limpias. |
| `embed` | `web` | Empaqueta las plantillas HTML dentro del binario compilado. Elimina la dependencia de archivos externos en runtime: un solo binario contiene código, lógica y frontend. |
| `strconv` | `web` | Conversión de parámetros HTTP (string a int) para páginas y IDs. Usado solo en la capa de transporte. |

---
## Principios de programación funcional aplicados

1. **Datos inmutables** — `store.Add`, `store.Update` y `store.Delete` operan sobre copias del slice original. Ninguna función modifica su entrada.
2. **Funciones puras** — Todas las funciones del núcleo (`models`, `store`, `search`, `reading`) producen el mismo resultado para los mismos argumentos y no tienen efectos secundarios.
3. **Funciones de orden superior** — `store.Update` recibe `func(Book) Book`; `search.Apply` recibe `...Filter` donde `Filter = func(Book) bool`. Esto permite componer comportamiento sin herencia ni interfaces.
4. **Efectos secundarios confinados** — Solo los paquetes `persistence`, `cli` y `web` interactúan con el sistema de archivos, la consola y la red. El núcleo del dominio (`models`, `store`, `search`, `reading`) es completamente puro.
5. **Cierre sobre estado externo** — Las closures `save` (CLI) y los handlers HTTP (web) capturan las dependencias (`repo`, `storage`) en lugar de usar variables globales. Esto facilita pruebas: basta inyectar implementaciones en memoria.

---

## Ejecución

### Modo línea de comandos

```bash
# Construir
go build -o sgb .

# Uso
./sgb add -title "Clean Code" -author "Robert C. Martin" -genre "Programación" -format PDF -isbn "9780132350884" -pages 464
./sgb list
./sgb search -author "Martin"
./sgb search -format PDF -status POR_LEER
./sgb reading start -id ABC123
./sgb reading progress -id ABC123 -page 150
./sgb reading finish -id ABC123
./sgb info -id ABC123
./sgb delete -id ABC123
```

### Modo web (interfaz gráfica)

```bash
# Iniciar servidor en http://localhost:8080
go run . -web

# Puerto personalizado
go run . -web -port :3000

# Construir y ejecutar
go build -o sgb .
./sgb -web
```

La interfaz web ofrece:
- Catálogo con búsqueda integrada por título, autor, formato y estado.
- Formulario de alta de libros con validación.
- Ficha detallada con progreso de lectura.
- Botones de acción: iniciar lectura, actualizar página, finalizar, eliminar.
- Mensajes flash de confirmación tras cada operación.
- Diseño responsive (escritorio y móvil).
