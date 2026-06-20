# Sistema de Gestión de E-Books

Aplicativo en Go para administrar una biblioteca personal de libros digitales. El proyecto fue migrado desde un enfoque funcional hacia una arquitectura orientada a objetos usando los mecanismos propios de Go: structs con métodos, constructores, interfaces, encapsulación por paquetes y objetos de servicio.

El sistema mantiene dos formas de uso:

- CLI para registrar, consultar, buscar, eliminar y gestionar lecturas desde consola.
- Servidor web con páginas HTML y servicios JSON para integrar el catálogo con clientes externos.

## Funcionalidades principales

- Registro de e-books con título, autor, género, formato, ISBN y número de páginas.
- Catálogo completo con búsqueda por título, autor, género, formato y estado de lectura.
- Detalle individual de cada libro.
- Eliminación de libros por ID.
- Ciclo de lectura: `POR_LEER`, `LEYENDO`, `LEIDO`.
- Actualización de página actual y cálculo de porcentaje de avance.
- Persistencia local en `books.json`.
- API web JSON con más de 8 servicios de distintas funcionalidades.
- Acceso concurrente protegido para el repositorio en memoria y la escritura del archivo JSON.

## Estructura del proyecto

```text
sistema-gestion/
├── main.go
├── books.json
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
        ├── list.html
        ├── add.html
        └── info.html
```

## Migración a POO

La migración no usa herencia clásica porque Go no trabaja con clases. En su lugar, aplica POO con composición, métodos e interfaces.

| Concepto | Implementación |
|---|---|
| Objetos | `models.Book`, `store.InMemoryRepository`, `persistence.JSONStorage`, `reading.DefaultReadingService`, `web.Server`. |
| Métodos | `Book.ProgressPercent()`, `Book.IsToRead()`, `Repository.Add()`, `Repository.Update()`, `Server.Start()`, handlers HTTP como métodos de `Server`. |
| Encapsulación | Campos internos no exportados: `InMemoryRepository.books`, `InMemoryRepository.mu`, `JSONStorage.path`, `Server.repo`, `Server.storage`. |
| Constructores | `models.NewBook`, `store.NewInMemoryRepository`, `persistence.NewJSONStorage`, `reading.NewReadingService`, `web.NewServer`. |
| Interfaces | `store.Repository`, `persistence.Storage`, `reading.ReadingService`, `search.Searcher`. |
| Concurrencia | `sync.RWMutex` protege el catálogo en memoria; `sync.Mutex` protege la lectura/escritura del archivo JSON. |
| Web | `net/http`, `html/template`, `embed` y servicios JSON REST-like. |

## Paquetes principales

### `models`

Define el objeto central `Book`, los formatos soportados (`PDF`, `EPUB`, `MOBI`, `AZW3`) y los estados de lectura. El constructor `NewBook` centraliza validaciones para evitar libros incompletos o con formatos inválidos.

También contiene comportamiento del objeto:

- `ProgressPercent()` calcula el avance.
- `IsToRead()` identifica libros pendientes.
- `IsReading()` identifica libros en lectura.
- `IsFinished()` identifica libros finalizados.

### `store`

Define la interfaz `Repository` y la implementación `InMemoryRepository`. El repositorio encapsula el slice de libros y solo permite acceso mediante métodos.

Métodos principales:

- `Add(book)`
- `FindByID(id)`
- `Update(id, updateFn)`
- `Delete(id)`
- `ListAll()`

El repositorio usa `sync.RWMutex` porque el servidor web atiende múltiples solicitudes al mismo tiempo.

### `reading`

Contiene la interfaz `ReadingService` y la implementación `DefaultReadingService`. Este servicio controla las reglas del ciclo de lectura:

- iniciar lectura,
- actualizar progreso,
- finalizar lectura,
- calcular porcentaje de avance.

### `search`

Define `Searcher` e `InMemorySearcher`. Mantiene filtros componibles para buscar libros por autor, título, género, formato y estado.

### `persistence`

Define la interfaz `Storage` y la implementación `JSONStorage`. Guarda y carga el catálogo en `books.json`. La ruta del archivo está encapsulada y las operaciones de archivo usan mutex para evitar escrituras concurrentes.

### `web`

Expone la interfaz HTML y los servicios web JSON. El objeto `Server` recibe sus dependencias por constructor, por lo que no depende de variables globales.

## Servicios web JSON

Inicia el servidor:

```bash
go run . -web
```

URL base:

```text
http://localhost:8080
```

| # | Método | Ruta | Funcionalidad |
|---|---|---|---|
| 1 | `GET` | `/api/books` | Lista todos los libros. |
| 2 | `POST` | `/api/books` | Registra un nuevo e-book. |
| 3 | `GET` | `/api/books/search?title=&author=&genre=&format=&status=` | Busca libros con filtros combinables. |
| 4 | `GET` | `/api/books/detail?id=ID` | Devuelve la ficha completa de un libro. |
| 5 | `DELETE` | `/api/books/delete?id=ID` | Elimina un libro del catálogo. |
| 6 | `POST` | `/api/reading/start` | Inicia la lectura de un libro. |
| 7 | `PUT` | `/api/reading/progress` | Actualiza la página actual. |
| 8 | `POST` | `/api/reading/finish` | Marca el libro como leído. |
| 9 | `GET` | `/api/reports/stats` | Genera métricas del catálogo. |
| 10 | `GET` | `/api/recommendations/next?limit=3` | Recomienda próximos libros pendientes. |

### Ejemplos de uso de la API

Crear un libro:

```bash
curl.exe -X POST http://localhost:8080/api/books ^
  -H "Content-Type: application/json" ^
  -d "{\"title\":\"Clean Code\",\"author\":\"Robert C. Martin\",\"genre\":\"Programación\",\"format\":\"PDF\",\"isbn\":\"9780132350884\",\"pages\":464}"
```

Buscar por autor:

```bash
curl.exe "http://localhost:8080/api/books/search?author=Martin"
```

Iniciar lectura:

```bash
curl.exe -X POST http://localhost:8080/api/reading/start ^
  -H "Content-Type: application/json" ^
  -d "{\"id\":\"ID_DEL_LIBRO\"}"
```

Actualizar progreso:

```bash
curl.exe -X PUT http://localhost:8080/api/reading/progress ^
  -H "Content-Type: application/json" ^
  -d "{\"id\":\"ID_DEL_LIBRO\",\"page\":120}"
```

Consultar estadísticas:

```bash
curl.exe http://localhost:8080/api/reports/stats
```

## Interfaz web HTML

Rutas disponibles:

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/` | Catálogo principal con búsqueda. |
| `GET` | `/books/add` | Formulario de registro. |
| `POST` | `/books/add` | Guarda un nuevo libro. |
| `GET` | `/books/info?id=ID` | Detalle del libro. |
| `POST` | `/books/delete` | Elimina un libro desde la interfaz. |
| `POST` | `/books/reading/start` | Inicia lectura. |
| `POST` | `/books/reading/progress` | Actualiza progreso. |
| `POST` | `/books/reading/finish` | Finaliza lectura. |

## Uso por CLI

```bash
go run . add -title "Clean Code" -author "Robert C. Martin" -genre "Programación" -format PDF -isbn "9780132350884" -pages 464
go run . list
go run . search -author "Martin"
go run . search -format PDF -status POR_LEER
go run . reading start -id ID_DEL_LIBRO
go run . reading progress -id ID_DEL_LIBRO -page 150
go run . reading finish -id ID_DEL_LIBRO
go run . info -id ID_DEL_LIBRO
go run . delete -id ID_DEL_LIBRO
```

## Ejecución

Modo consola:

```bash
go run . list
```

Modo web:

```bash
go run . -web
```

Puerto personalizado:

```bash
go run . -web -port :3000
```

Compilar:

```bash
go build -o sgb .
```

## Verificación

El proyecto compila y ejecuta pruebas de paquetes con:

```bash
go test ./...
```

En entornos con restricciones de permisos sobre la caché de Go, se puede usar una caché temporal:

```powershell
$env:GOCACHE = Join-Path $env:TEMP 'codex-go-build-sistema-gestion'
go test ./...
```
