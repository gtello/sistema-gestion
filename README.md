# Sistema de Gestión de E-Books

Sistema de línea de comandos para administrar una biblioteca personal de libros digitales. Desarrollado en Go bajo el paradigma de **programación funcional**: cero métodos en structs, inmutabilidad de datos y funciones de orden superior como mecanismo principal de composición.

## Alcance del sistema

- Registrar libros con metadatos completos: título, autor, género, formato, ISBN y número de páginas.
- Consultar el catálogo completo o ver el detalle de un libro específico.
- Buscar libros combinando múltiples criterios de filtrado.
- Gestionar el ciclo de vida de lectura: por leer → leyendo → leído, con seguimiento de progreso por página.
- Persistir los datos en archivos JSON para que sobrevivan entre ejecuciones.

**Fuera de alcance:** edición del contenido del libro, renderizado de formatos, sincronización con dispositivos externos, autenticación de usuarios, interfaz gráfica.

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
└── cli/
    └── cli.go
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

Implementará las operaciones de almacenamiento en memoria sobre un slice de `Book`. Cada función recibe el estado actual y devuelve un estado nuevo, sin mutar el original.

### `search`: Filtros componibles

Definirá `Filter` como `func(Book) bool` y provee constructores de filtros que se combinan con `Apply`.

### `reading`: Ciclo de lectura

Gestionará las transiciones de estado del proceso de lectura usando las funciones de `store` como base.

### `persistence`: Serialización JSON

Capa de entrada/salida que convierte entre el slice en memoria y el archivo en disco.

### `cli`: Interfaz de línea de comandos

Orquestrará todos los módulos anteriores. Usa el paquete `flag` de la biblioteca estándar para parsear subcomandos y opciones.

---

## Paquetes incorporados usados

| Paquete | Uso | Justificación |
|---------|-----|---------------|
| `encoding/json` | `persistence` | `MarshalIndent` y `Unmarshal` para serializar/deserializar el catálogo a JSON legible. Alternativas como `gob` o `protobuf` son binarias y no permiten inspeccionar el archivo manualmente. |
| `os` | `persistence`, `cli` | `ReadFile` y `WriteFile` para E/S de archivos; `os.Args` para leer argumentos de línea de comandos; `os.Stderr` para mensajes de error. La biblioteca estándar cubre completamente estas necesidades sin dependencias externas. |
| `flag` | `cli` | Parseo de subcomandos y banderas con `FlagSet`. Elegido sobre `os.Args` manual porque maneja automáticamente la validación de tipos, valores por defecto y mensajes de ayuda. No se usó `cobra` ni `urfave/cli` para mantener cero dependencias externas, alineado con el principio de simplicidad del proyecto. |
| `time` | `models`, `reading`, `cli` | Registro de fechas de alta, inicio de lectura y finalización. `time.Time` ofrece comparación, formateo y serialización JSON sin lógica adicional. |
| `errors` | `store`, `reading` | Creación de errores descriptivos con `errors.New`. Retorna `error` como segundo valor de retorno (convención Go) en lugar de usar `panic`, que detendría la ejecución. |
| `strings` | `search` | `Contains` y `ToLower` para búsquedas parciales case-insensitive por autor y título. |
| `fmt` | `cli` | Salida formateada a consola (`Printf`, `Sprintf`). Usado exclusivamente en la capa de presentación. |

