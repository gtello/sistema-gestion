package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"sistema-gestion/models"
	"sistema-gestion/persistence"
	"sistema-gestion/reading"
	"sistema-gestion/search"
	"sistema-gestion/store"
)

const dataFile = "books.json"

// RunWithDeps ejecuta la CLI con dependencias inyectadas. main.go construye
// las dependencias una sola vez y las comparte entre CLI y Web según el flag -web.
// Esto evita duplicar la inicialización de storage, repo y servicios.
func RunWithDeps(repo store.Repository, storage persistence.Storage, readingSvc reading.ReadingService, searcher search.Searcher) {
	// La closure save encapsula la lógica de persistencia. Usa ListAll() de la
	// interfaz Repository en lugar de la implementación concreta.
	save := func() {
		if err := storage.Save(repo.ListAll()); err != nil {
			fmt.Fprintf(os.Stderr, "Error al guardar datos: %v\n", err)
			os.Exit(1)
		}
	}

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "add":
		cmdAdd(repo, save)
	case "list":
		cmdList(repo)
	case "search":
		cmdSearch(repo, searcher)
	case "reading":
		cmdReading(repo, readingSvc, save)
	case "delete":
		cmdDelete(repo, save)
	case "info":
		cmdInfo(repo, readingSvc)
	default:
		fmt.Fprintf(os.Stderr, "Comando desconocido: %s\n", os.Args[1])
		printUsage()
	}
}

// Run inicializa las dependencias localmente y delega en RunWithDeps.
// Mantenida para uso directo (go run . sin argumentos de web).
func Run() {
	storage := persistence.NewJSONStorage(dataFile)
	books, err := storage.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error al cargar datos: %v\n", err)
		os.Exit(1)
	}

	repo := store.NewInMemoryRepository(books)
	readingSvc := reading.NewReadingService()
	searcher := &search.InMemorySearcher{}

	RunWithDeps(repo, storage, readingSvc, searcher)
}

func printUsage() {
	fmt.Println(`Uso: sistema-gestion <comando> [opciones]

Comandos:
  add       Añadir un nuevo libro
  list      Listar todos los libros
  search    Buscar libros con filtros
  reading   Gestionar progreso de lectura
  delete    Eliminar un libro
  info      Ver detalles de un libro

Ejemplos:
  sistema-gestion add -title "Clean Code" -author "R. Martin" -genre "Programación" -format PDF -isbn "123" -pages 464
  sistema-gestion list
  sistema-gestion search -author "Martin"
  sistema-gestion reading start -id <id>
  sistema-gestion reading progress -id <id> -page 120
  sistema-gestion reading finish -id <id>
  sistema-gestion delete -id <id>
  sistema-gestion info -id <id>`)
}

func cmdAdd(repo store.Repository, save func()) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	title := fs.String("title", "", "Título del libro")
	author := fs.String("author", "", "Autor del libro")
	genre := fs.String("genre", "", "Género del libro")
	format := fs.String("format", "", "Formato (PDF, EPUB, MOBI, AZW3)")
	isbn := fs.String("isbn", "", "ISBN del libro")
	pages := fs.Int("pages", 0, "Número de páginas")

	fs.Parse(os.Args[2:])

	if *title == "" || *author == "" {
		fmt.Println("Error: -title y -author son obligatorios")
		fs.Usage()
		return
	}

	// Se usa el constructor NewBook que encapsula la validación.
	// El CLI no asigna campos manualmente; delega la integridad en models.
	book, err := models.NewBook(
		generateID(*title, *author),
		*title,
		*author,
		*genre,
		parseFormat(*format),
		*isbn,
		*pages,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error de validación: %v\n", err)
		return
	}

	// Se usa la interfaz Repository, no el struct concreto.
	// Esto permite cambiar la implementación sin tocar la CLI.
	if err := repo.Add(book); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fmt.Fprintf(os.Stderr, "Error: ya existe un libro con ID %s\n", book.ID)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return
	}

	save()
	fmt.Printf("Libro añadido: [%s] %s - %s\n", book.ID, book.Title, book.Author)
}

func cmdList(repo store.Repository) {
	books := repo.ListAll()

	if len(books) == 0 {
		fmt.Println("No hay libros registrados.")
		return
	}

	printBookTable(books)
}

func cmdSearch(repo store.Repository, searcher search.Searcher) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	author := fs.String("author", "", "Buscar por autor")
	title := fs.String("title", "", "Buscar por título")
	genre := fs.String("genre", "", "Buscar por género")
	format := fs.String("format", "", "Buscar por formato")
	status := fs.String("status", "", "Buscar por estado (POR_LEER, LEYENDO, LEIDO)")

	fs.Parse(os.Args[2:])

	books := repo.ListAll()
	var filters []search.Filter

	// Cada filtro se construye como función pura y se acumula en un slice.
	// Se aplican todos en conjunción (AND) al llamar a Search.
	if *author != "" {
		filters = append(filters, search.ByAuthor(*author))
	}
	if *title != "" {
		filters = append(filters, search.ByTitle(*title))
	}
	if *genre != "" {
		filters = append(filters, search.ByGenre(*genre))
	}
	if *format != "" {
		filters = append(filters, search.ByFormat(parseFormat(*format)))
	}
	if *status != "" {
		filters = append(filters, search.ByStatus(parseStatus(*status)))
	}

	results := searcher.Search(books, filters...)

	if len(results) == 0 {
		fmt.Println("No se encontraron libros con esos criterios.")
		return
	}

	fmt.Printf("Resultados: %d libro(s)\n\n", len(results))
	printBookTable(results)
}

func cmdReading(repo store.Repository, readingSvc reading.ReadingService, save func()) {
	if len(os.Args) < 3 {
		fmt.Println("Uso: sistema-gestion reading <start|progress|finish> [opciones]")
		return
	}

	sub := os.Args[2]

	switch sub {
	case "start":
		fs := flag.NewFlagSet("reading start", flag.ExitOnError)
		id := fs.String("id", "", "ID del libro")
		fs.Parse(os.Args[3:])

		if *id == "" {
			fmt.Println("Error: -id es obligatorio")
			return
		}

		if err := readingSvc.StartReading(repo, *id, time.Now()); err != nil {
			// Se identifican errores de negocio con errors.Is para dar
			// mensajes específicos al usuario según la causa raíz.
			switch {
			case errors.Is(err, reading.ErrBookNotFound):
				fmt.Fprintf(os.Stderr, "Error: libro con ID %s no encontrado\n", *id)
			case errors.Is(err, reading.ErrAlreadyReading):
				fmt.Fprintf(os.Stderr, "Error: el libro ya está en estado LEYENDO\n")
			case errors.Is(err, reading.ErrAlreadyFinished):
				fmt.Fprintf(os.Stderr, "Error: el libro ya fue leído\n")
			default:
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			return
		}
		save()
		fmt.Println("Lectura iniciada.")

	case "progress":
		fs := flag.NewFlagSet("reading progress", flag.ExitOnError)
		id := fs.String("id", "", "ID del libro")
		page := fs.Int("page", 0, "Página actual")
		fs.Parse(os.Args[3:])

		if *id == "" {
			fmt.Println("Error: -id es obligatorio")
			return
		}

		if err := readingSvc.UpdateProgress(repo, *id, *page); err != nil {
			switch {
			case errors.Is(err, reading.ErrBookNotFound):
				fmt.Fprintf(os.Stderr, "Error: libro con ID %s no encontrado\n", *id)
			case errors.Is(err, reading.ErrNotReading):
				fmt.Fprintf(os.Stderr, "Error: solo se puede actualizar progreso de un libro en estado LEYENDO\n")
			case errors.Is(err, reading.ErrInvalidPage):
				fmt.Fprintf(os.Stderr, "Error: la página actual no puede ser negativa\n")
			case errors.Is(err, reading.ErrPageExceedsTotal):
				fmt.Fprintf(os.Stderr, "Error: página fuera del rango del libro\n")
			default:
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			return
		}
		save()

		pct, _ := readingSvc.GetProgress(repo, *id)
		fmt.Printf("Progreso actualizado: página %d (%.1f%%)\n", *page, pct)

	case "finish":
		fs := flag.NewFlagSet("reading finish", flag.ExitOnError)
		id := fs.String("id", "", "ID del libro")
		fs.Parse(os.Args[3:])

		if *id == "" {
			fmt.Println("Error: -id es obligatorio")
			return
		}

		if err := readingSvc.FinishReading(repo, *id, time.Now()); err != nil {
			switch {
			case errors.Is(err, reading.ErrBookNotFound):
				fmt.Fprintf(os.Stderr, "Error: libro con ID %s no encontrado\n", *id)
			case errors.Is(err, reading.ErrAlreadyFinished):
				fmt.Fprintf(os.Stderr, "Error: el libro ya fue leído\n")
			case errors.Is(err, reading.ErrNotReading):
				fmt.Fprintf(os.Stderr, "Error: debe iniciar la lectura antes de finalizarla\n")
			default:
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			return
		}
		save()
		fmt.Println("Lectura finalizada.")

	default:
		fmt.Printf("Subcomando desconocido: %s\n", sub)
		fmt.Println("Use: start, progress, finish")
	}
}

func cmdDelete(repo store.Repository, save func()) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	id := fs.String("id", "", "ID del libro a eliminar")
	fs.Parse(os.Args[2:])

	if *id == "" {
		fmt.Println("Error: -id es obligatorio")
		return
	}

	// Se obtiene el libro antes de eliminarlo para mostrar sus datos.
	book, err := repo.FindByID(*id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Error: libro con ID %s no encontrado\n", *id)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return
	}

	if err := repo.Delete(*id); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	save()
	fmt.Printf("Libro eliminado: [%s] %s - %s\n", book.ID, book.Title, book.Author)
}

func cmdInfo(repo store.Repository, readingSvc reading.ReadingService) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	id := fs.String("id", "", "ID del libro")
	fs.Parse(os.Args[2:])

	if *id == "" {
		fmt.Println("Error: -id es obligatorio")
		return
	}

	book, err := repo.FindByID(*id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Error: libro con ID %s no encontrado\n", *id)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return
	}

	fmt.Println("──────────────────────────────")
	fmt.Printf("ID:         %s\n", book.ID)
	fmt.Printf("Título:     %s\n", book.Title)
	fmt.Printf("Autor:      %s\n", book.Author)
	fmt.Printf("Género:     %s\n", book.Genre)
	fmt.Printf("Formato:    %s\n", book.Format)
	fmt.Printf("ISBN:       %s\n", book.ISBN)
	fmt.Printf("Páginas:    %d\n", book.Pages)
	fmt.Printf("Pág. actual:%d\n", book.CurrentPage)
	fmt.Printf("Estado:     %s\n", book.Status)

	// Solo se muestra progreso si la lectura está en curso o finalizada.
	if book.Status == models.StatusReading || book.Status == models.StatusFinished {
		pct, _ := readingSvc.GetProgress(repo, *id)
		fmt.Printf("Progreso:   %.1f%%\n", pct)
	}

	if book.StartedAt != nil {
		fmt.Printf("Iniciado:   %s\n", book.StartedAt.Format("2006-01-02"))
	}
	if book.FinishedAt != nil {
		fmt.Printf("Finalizado: %s\n", book.FinishedAt.Format("2006-01-02"))
	}
	fmt.Printf("Añadido:    %s\n", book.AddedAt.Format("2006-01-02"))
	fmt.Println("──────────────────────────────")
}

// printBookTable muestra los libros en formato tabular.
// Usa truncate para evitar que columnas largas rompan el layout.
func printBookTable(books []models.Book) {
	fmt.Printf("%-10s %-25s %-20s %-15s %-8s %-12s\n", "ID", "TÍTULO", "AUTOR", "GÉNERO", "FORMATO", "ESTADO")
	fmt.Println("────────── ──────────────────────── ──────────────────── ─────────────── ──────── ────────────")
	for _, b := range books {
		fmt.Printf("%-10s %-25s %-20s %-15s %-8s %-12s\n",
			b.ID, truncate(b.Title, 25), truncate(b.Author, 20), truncate(b.Genre, 15), b.Format, b.Status)
	}
}

func parseFormat(s string) models.BookFormat {
	switch s {
	case "EPUB":
		return models.FormatEPUB
	case "MOBI":
		return models.FormatMOBI
	case "AZW3":
		return models.FormatAZW3
	default:
		return models.FormatPDF
	}
}

func parseStatus(s string) models.ReadingStatus {
	switch s {
	case "LEYENDO":
		return models.StatusReading
	case "LEIDO":
		return models.StatusFinished
	default:
		return models.StatusToRead
	}
}

// generateID produce un identificador alfanumérico de 8 caracteres a partir
// de la marca de tiempo actual combinada con el título y el autor.
// No es criptográficamente seguro; su propósito es generar claves únicas
// legibles para un catálogo personal.
func generateID(title, author string) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	now := time.Now().UnixMilli()
	id := make([]byte, 8)
	for i := range id {
		// La fórmula mezcla timestamp + título + autor para distribuir
		// los caracteres y reducir colisiones entre libros añadidos
		// en rápida sucesión.
		id[i] = chars[(int(now)+i*len(title)*len(author))%len(chars)]
	}
	return string(id)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-2] + ".."
}
