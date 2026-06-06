package web

import (
	"embed"
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

	fmt.Printf("Servidor web iniciado en http://localhost%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) save() error {
	return s.storage.Save(s.repo.ListAll())
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

func generateWebID(title, author string) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	now := time.Now().UnixMilli()
	id := make([]byte, 8)
	for i := range id {
		id[i] = chars[(int(now)+i*len(title)*len(author))%len(chars)]
	}
	return string(id)
}
