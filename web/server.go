package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"sistema-gestion/models"
	"sistema-gestion/reading"
	"sistema-gestion/search"
	"sistema-gestion/store"
)

//go:embed templates/*
var templateFS embed.FS

// Server encapsula el servidor HTTP y sus dependencias.
type Server struct {
	repo       store.Repository
	storage    persistenceStorage
	readingSvc reading.ReadingService
	searcher   search.Searcher
	templates  *template.Template
}

// persistenceStorage es la interfaz que necesita el servidor para guardar.
// Evita import circular permitiendo que main.go inyecte la dependencia.
type persistenceStorage interface {
	Save([]models.Book) error
}

// NewServer construye el servidor web y compila las plantillas HTML.
func NewServer(repo store.Repository, storage persistenceStorage, readingSvc reading.ReadingService, searcher search.Searcher) (*Server, error) {
	funcMap := template.FuncMap{
		"statusBadge": func(s models.ReadingStatus) template.HTML {
			switch s {
			case models.StatusToRead:
				return `<span class="badge badge-unread">POR LEER</span>`
			case models.StatusReading:
				return `<span class="badge badge-reading">LEYENDO</span>`
			case models.StatusFinished:
				return `<span class="badge badge-read">LEÍDO</span>`
			}
			return ""
		},
		"percent": func(current, total int) int {
			if total == 0 {
				return 0
			}
			return current * 100 / total
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("error al compilar plantillas: %w", err)
	}

	return &Server{
		repo:       repo,
		storage:    storage,
		readingSvc: readingSvc,
		searcher:   searcher,
		templates:  tmpl,
	}, nil
}

// Start inicia el servidor HTTP.
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleList)
	mux.HandleFunc("/books/add", s.handleAdd)
	mux.HandleFunc("/books/info", s.handleInfo)
	mux.HandleFunc("/books/delete", s.handleDelete)
	mux.HandleFunc("/books/reading/start", s.handleReadingStart)
	mux.HandleFunc("/books/reading/progress", s.handleReadingProgress)
	mux.HandleFunc("/books/reading/finish", s.handleReadingFinish)

	mux.HandleFunc("/api/books", s.handleAPIBooks)
	mux.HandleFunc("/api/books/search", s.handleAPIBookSearch)
	mux.HandleFunc("/api/books/detail", s.handleAPIBookDetail)
	mux.HandleFunc("/api/books/delete", s.handleAPIBookDelete)
	mux.HandleFunc("/api/reading/start", s.handleAPIReadingStart)
	mux.HandleFunc("/api/reading/progress", s.handleAPIReadingProgress)
	mux.HandleFunc("/api/reading/finish", s.handleAPIReadingFinish)
	mux.HandleFunc("/api/reports/stats", s.handleAPIStats)
	mux.HandleFunc("/api/recommendations/next", s.handleAPIRecommendations)

	fmt.Printf("Servidor web iniciado en http://localhost%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) save() error {
	return s.storage.Save(s.repo.ListAll())
}

type apiResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

type bookInput struct {
	ID     string `json:"id,omitempty"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Genre  string `json:"genre"`
	Format string `json:"format"`
	ISBN   string `json:"isbn"`
	Pages  int    `json:"pages"`
}

type idInput struct {
	ID string `json:"id"`
}

type progressInput struct {
	ID   string `json:"id"`
	Page int    `json:"page"`
}

type catalogStats struct {
	TotalBooks      int            `json:"total_books"`
	TotalPages      int            `json:"total_pages"`
	ByStatus        map[string]int `json:"by_status"`
	ByFormat        map[string]int `json:"by_format"`
	AverageProgress float64        `json:"average_progress"`
}

func writeAPIData(w http.ResponseWriter, status int, data interface{}) {
	writeAPIResponse(w, status, apiResponse{OK: true, Data: data})
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeAPIResponse(w, status, apiResponse{OK: false, Error: message})
}

func writeAPIResponse(w http.ResponseWriter, status int, response apiResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

func decodeAPIJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func apiFiltersFromRequest(r *http.Request) []search.Filter {
	query := r.URL.Query()
	var filters []search.Filter

	if title := query.Get("title"); title != "" {
		filters = append(filters, search.ByTitle(title))
	}
	if author := query.Get("author"); author != "" {
		filters = append(filters, search.ByAuthor(author))
	}
	if genre := query.Get("genre"); genre != "" {
		filters = append(filters, search.ByGenre(genre))
	}
	if format := query.Get("format"); format != "" {
		filters = append(filters, search.ByFormat(models.BookFormat(format)))
	}
	if status := query.Get("status"); status != "" {
		filters = append(filters, search.ByStatus(models.ReadingStatus(status)))
	}

	return filters
}

func writeAPIReadingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, reading.ErrBookNotFound):
		writeAPIError(w, http.StatusNotFound, "libro no encontrado")
	case errors.Is(err, reading.ErrAlreadyReading):
		writeAPIError(w, http.StatusConflict, "el libro ya está en estado LEYENDO")
	case errors.Is(err, reading.ErrAlreadyFinished):
		writeAPIError(w, http.StatusConflict, "el libro ya fue leído")
	case errors.Is(err, reading.ErrNotReading):
		writeAPIError(w, http.StatusBadRequest, "debe iniciar la lectura primero")
	case errors.Is(err, reading.ErrInvalidPage):
		writeAPIError(w, http.StatusBadRequest, "página no válida")
	case errors.Is(err, reading.ErrPageExceedsTotal):
		writeAPIError(w, http.StatusBadRequest, "página fuera del rango del libro")
	default:
		writeAPIError(w, http.StatusInternalServerError, "error interno")
	}
}

// --- Handlers ---

// handleList: GET / — catálogo principal con búsqueda.
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	books := s.repo.ListAll()
	var filters []search.Filter

	searchTitle := r.URL.Query().Get("title")
	searchAuthor := r.URL.Query().Get("author")
	searchFormat := r.URL.Query().Get("format")
	searchStatus := r.URL.Query().Get("status")
	isSearch := searchTitle != "" || searchAuthor != "" || searchFormat != "" || searchStatus != ""

	if searchTitle != "" {
		filters = append(filters, search.ByTitle(searchTitle))
	}
	if searchAuthor != "" {
		filters = append(filters, search.ByAuthor(searchAuthor))
	}
	if searchFormat != "" {
		filters = append(filters, search.ByFormat(models.BookFormat(searchFormat)))
	}
	if searchStatus != "" {
		filters = append(filters, search.ByStatus(models.ReadingStatus(searchStatus)))
	}

	results := s.searcher.Search(books, filters...)

	flash := r.URL.Query().Get("flash")
	flashSuccess := r.URL.Query().Get("flash_type") != "error"

	s.templates.ExecuteTemplate(w, "list.html", map[string]interface{}{
		"Books":        results,
		"Flash":        flash,
		"FlashSuccess": flashSuccess,
		"SearchTitle":  searchTitle,
		"SearchAuthor": searchAuthor,
		"SearchFormat": searchFormat,
		"SearchStatus": searchStatus,
		"IsSearch":     isSearch,
	})
}

// handleAdd: GET /books/add → formulario; POST → guarda libro.
func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.templates.ExecuteTemplate(w, "add.html", map[string]interface{}{
			"Title":  "",
			"Author": "",
			"Genre":  "",
			"ISBN":   "",
			"Pages":  0,
			"Format": "PDF",
		})
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			s.templates.ExecuteTemplate(w, "add.html", map[string]interface{}{"Flash": "Error al procesar el formulario", "FlashSuccess": false})
			return
		}

		pages, _ := strconv.Atoi(r.FormValue("pages"))
		id := generateWebID(r.FormValue("title"), r.FormValue("author"))

		book, err := models.NewBook(
			id,
			r.FormValue("title"),
			r.FormValue("author"),
			r.FormValue("genre"),
			models.BookFormat(r.FormValue("format")),
			r.FormValue("isbn"),
			pages,
		)
		if err != nil {
			s.templates.ExecuteTemplate(w, "add.html", map[string]interface{}{
				"Flash":        err.Error(),
				"FlashSuccess": false,
				"Title":        r.FormValue("title"),
				"Author":       r.FormValue("author"),
				"Genre":        r.FormValue("genre"),
				"ISBN":         r.FormValue("isbn"),
				"Pages":        pages,
				"Format":       r.FormValue("format"),
			})
			return
		}

		if err := s.repo.Add(book); err != nil {
			msg := "Error al añadir el libro"
			if errors.Is(err, store.ErrDuplicate) {
				msg = "Ya existe un libro con ese ID"
			}
			s.templates.ExecuteTemplate(w, "add.html", map[string]interface{}{
				"Flash": msg, "FlashSuccess": false,
				"Title": r.FormValue("title"), "Author": r.FormValue("author"),
				"Genre": r.FormValue("genre"), "ISBN": r.FormValue("isbn"),
				"Pages": pages, "Format": r.FormValue("format"),
			})
			return
		}

		s.save()
		http.Redirect(w, r, "/?flash=Libro+añadido+correctamente&flash_type=success", http.StatusSeeOther)
	}
}

// handleInfo: GET /books/info?id=X — ficha detallada del libro.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Redirect(w, r, "/?flash=ID+no+especificado&flash_type=error", http.StatusSeeOther)
		return
	}

	book, err := s.repo.FindByID(id)
	if err != nil {
		http.Redirect(w, r, "/?flash=Libro+no+encontrado&flash_type=error", http.StatusSeeOther)
		return
	}

	flash := r.URL.Query().Get("flash")
	s.templates.ExecuteTemplate(w, "info.html", map[string]interface{}{
		"Book":  book,
		"Flash": flash,
	})
}

// handleDelete: POST /books/delete — elimina un libro.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	id := r.FormValue("id")

	if err := s.repo.Delete(id); err != nil {
		http.Redirect(w, r, "/?flash=Error+al+eliminar&flash_type=error", http.StatusSeeOther)
		return
	}

	s.save()
	http.Redirect(w, r, "/?flash=Libro+eliminado&flash_type=success", http.StatusSeeOther)
}

// handleReadingStart: POST /books/reading/start — inicia lectura.
func (s *Server) handleReadingStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	id := r.FormValue("id")

	if err := s.readingSvc.StartReading(s.repo, id, time.Now()); err != nil {
		msg := "Error al iniciar lectura"
		if errors.Is(err, reading.ErrAlreadyReading) {
			msg = "El libro ya está en estado LEYENDO"
		} else if errors.Is(err, reading.ErrAlreadyFinished) {
			msg = "El libro ya fue leído"
		}
		http.Redirect(w, r, "/?flash="+msg+"&flash_type=error", http.StatusSeeOther)
		return
	}

	s.save()
	http.Redirect(w, r, "/?flash=Lectura+iniciada&flash_type=success", http.StatusSeeOther)
}

// handleReadingProgress: POST /books/reading/progress — actualiza página.
func (s *Server) handleReadingProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	id := r.FormValue("id")
	page, _ := strconv.Atoi(r.FormValue("page"))
	redirect := r.FormValue("redirect")

	if err := s.readingSvc.UpdateProgress(s.repo, id, page); err != nil {
		msg := "Error al actualizar progreso"
		if errors.Is(err, reading.ErrNotReading) {
			msg = "El libro no está en estado LEYENDO"
		} else if errors.Is(err, reading.ErrInvalidPage) {
			msg = "Página no válida"
		} else if errors.Is(err, reading.ErrPageExceedsTotal) {
			msg = "Página fuera del rango del libro"
		}
		http.Redirect(w, r, "/?flash="+msg+"&flash_type=error", http.StatusSeeOther)
		return
	}

	s.save()
	if redirect == "info" {
		http.Redirect(w, r, "/books/info?id="+id+"&flash=Progreso+actualizado", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/?flash=Progreso+actualizado&flash_type=success", http.StatusSeeOther)
	}
}

// handleReadingFinish: POST /books/reading/finish — finaliza lectura.
func (s *Server) handleReadingFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	id := r.FormValue("id")

	if err := s.readingSvc.FinishReading(s.repo, id, time.Now()); err != nil {
		msg := "Error al finalizar lectura"
		if errors.Is(err, reading.ErrAlreadyFinished) {
			msg = "El libro ya fue leído"
		} else if errors.Is(err, reading.ErrNotReading) {
			msg = "Debe iniciar la lectura primero"
		}
		http.Redirect(w, r, "/?flash="+msg+"&flash_type=error", http.StatusSeeOther)
		return
	}

	s.save()
	http.Redirect(w, r, "/?flash=Lectura+finalizada&flash_type=success", http.StatusSeeOther)
}

// handleAPIBooks expone listado y alta de libros en JSON.
func (s *Server) handleAPIBooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeAPIData(w, http.StatusOK, s.repo.ListAll())
	case http.MethodPost:
		var input bookInput
		if err := decodeAPIJSON(r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "JSON inválido")
			return
		}

		format := input.Format
		if format == "" {
			format = string(models.FormatPDF)
		}

		id := input.ID
		if id == "" {
			id = generateWebID(input.Title, input.Author)
		}

		book, err := models.NewBook(
			id,
			input.Title,
			input.Author,
			input.Genre,
			models.BookFormat(format),
			input.ISBN,
			input.Pages,
		)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := s.repo.Add(book); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				writeAPIError(w, http.StatusConflict, "ya existe un libro con ese ID")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "error al añadir el libro")
			return
		}

		if err := s.save(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "error al guardar datos")
			return
		}

		writeAPIData(w, http.StatusCreated, book)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
	}
}

// handleAPIBookSearch busca libros por título, autor, género, formato y estado.
func (s *Server) handleAPIBookSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	books := s.repo.ListAll()
	results := s.searcher.Search(books, apiFiltersFromRequest(r)...)
	writeAPIData(w, http.StatusOK, results)
}

// handleAPIBookDetail devuelve la ficha completa de un libro.
func (s *Server) handleAPIBookDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "id requerido")
		return
	}

	book, err := s.repo.FindByID(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "libro no encontrado")
		return
	}

	writeAPIData(w, http.StatusOK, book)
}

// handleAPIBookDelete elimina un libro por ID.
func (s *Server) handleAPIBookDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "id requerido")
		return
	}

	if err := s.repo.Delete(id); err != nil {
		writeAPIError(w, http.StatusNotFound, "libro no encontrado")
		return
	}

	if err := s.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "error al guardar datos")
		return
	}

	writeAPIData(w, http.StatusOK, map[string]string{"id": id, "message": "libro eliminado"})
}

// handleAPIReadingStart inicia el ciclo de lectura desde JSON.
func (s *Server) handleAPIReadingStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	var input idInput
	if err := decodeAPIJSON(r, &input); err != nil || input.ID == "" {
		writeAPIError(w, http.StatusBadRequest, "id requerido")
		return
	}

	if err := s.readingSvc.StartReading(s.repo, input.ID, time.Now()); err != nil {
		writeAPIReadingError(w, err)
		return
	}

	if err := s.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "error al guardar datos")
		return
	}

	book, _ := s.repo.FindByID(input.ID)
	writeAPIData(w, http.StatusOK, book)
}

// handleAPIReadingProgress actualiza la página actual desde JSON.
func (s *Server) handleAPIReadingProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	var input progressInput
	if err := decodeAPIJSON(r, &input); err != nil || input.ID == "" {
		writeAPIError(w, http.StatusBadRequest, "id y page requeridos")
		return
	}

	if err := s.readingSvc.UpdateProgress(s.repo, input.ID, input.Page); err != nil {
		writeAPIReadingError(w, err)
		return
	}

	if err := s.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "error al guardar datos")
		return
	}

	book, _ := s.repo.FindByID(input.ID)
	writeAPIData(w, http.StatusOK, map[string]interface{}{
		"book":             book,
		"progress_percent": book.ProgressPercent(),
	})
}

// handleAPIReadingFinish marca un libro como leído desde JSON.
func (s *Server) handleAPIReadingFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	var input idInput
	if err := decodeAPIJSON(r, &input); err != nil || input.ID == "" {
		writeAPIError(w, http.StatusBadRequest, "id requerido")
		return
	}

	if err := s.readingSvc.FinishReading(s.repo, input.ID, time.Now()); err != nil {
		writeAPIReadingError(w, err)
		return
	}

	if err := s.save(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "error al guardar datos")
		return
	}

	book, _ := s.repo.FindByID(input.ID)
	writeAPIData(w, http.StatusOK, book)
}

// handleAPIStats resume el catálogo para paneles o integraciones externas.
func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	stats := catalogStats{
		ByStatus: map[string]int{
			string(models.StatusToRead):   0,
			string(models.StatusReading):  0,
			string(models.StatusFinished): 0,
		},
		ByFormat: map[string]int{
			string(models.FormatPDF):  0,
			string(models.FormatEPUB): 0,
			string(models.FormatMOBI): 0,
			string(models.FormatAZW3): 0,
		},
	}

	for _, book := range s.repo.ListAll() {
		stats.TotalBooks++
		stats.TotalPages += book.Pages
		stats.ByStatus[string(book.Status)]++
		stats.ByFormat[string(book.Format)]++
		stats.AverageProgress += book.ProgressPercent()
	}

	if stats.TotalBooks > 0 {
		stats.AverageProgress = stats.AverageProgress / float64(stats.TotalBooks)
	}

	writeAPIData(w, http.StatusOK, stats)
}

// handleAPIRecommendations recomienda los próximos libros pendientes.
func (s *Server) handleAPIRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "método no permitido")
		return
	}

	limit := 3
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 {
			writeAPIError(w, http.StatusBadRequest, "limit debe ser mayor a 0")
			return
		}
		limit = parsed
	}

	var recommendations []models.Book
	for _, book := range s.repo.ListAll() {
		if book.IsToRead() {
			recommendations = append(recommendations, book)
		}
	}

	if len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}

	writeAPIData(w, http.StatusOK, recommendations)
}

func generateWebID(title, author string) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	now := time.Now().UnixMilli()
	id := make([]byte, 8)
	for i := range id {
		id[i] = chars[(int(now)+i*len(title)*len(author))%len(chars)]
	}
	return string(id)
}
