package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"puush/database"
)

type Server struct {
	db    *database.Database
	muxer *http.ServeMux
	root  string
}

func Create(db *database.Database) (Server, error) {
	root := os.Getenv("PUUSH_ROOT_DIRECTORY")
	if root == "" {
		root = "/srv/puush"
	}

	if fileinfo, err := os.Stat(root); os.IsNotExist(err) {
		fmt.Printf("Creating directory '%s'\n", root)
		if err = os.MkdirAll(root, 0755); err != nil {
			return Server{}, err
		}
	} else if !fileinfo.IsDir() {
		return Server{}, errors.New("Specified root directory is a file")
	}

	s := Server{
		db:    db,
		muxer: http.NewServeMux(),
		root:  root,
	}

	s.muxer.Handle("/api/session", s.log(s.handleSession()))
	s.muxer.Handle("/api/upload", s.log(s.doesSessionExist(s.handleUpload())))
	s.muxer.Handle("/api/list", s.log(s.doesSessionExist(s.handleList())))
	s.muxer.Handle("/", s.log(s.handleFile()))

	return s, nil
}

func (s *Server) Serve() error {
	return http.ListenAndServe(":8080", s.muxer)
}

func (s *Server) getUrl(r *http.Request) string {
	protocol := r.Header.Get("X-Forwarded-Proto")
	if protocol == "" {
		protocol = "http"
	}

	return protocol + "://" + r.Host
}

func (s *Server) log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(fmt.Sprintf("%s %s", r.Method, r.URL.Path))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) doesSessionExist(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionCookie, err := r.Cookie("SESSION_KEY")
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("Missing session key cookie"))
			return
		}

		sessionKey := sessionCookie.Value
		sessionExists, err := s.db.DoesSessionExist(sessionKey)
		if err != nil || !sessionExists {
			w.WriteHeader(401)
			w.Write([]byte("Invalid session key cookie"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleSession() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(400)
			w.Write([]byte("Unsupported method"))
			return
		}

		sessionKey, err := s.db.AddSession()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			w.WriteHeader(500)
			w.Write([]byte("Internal server error"))
			return
		}

		w.WriteHeader(200)
		w.Write([]byte(sessionKey))
	})
}

func (s *Server) handleUpload() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(400)
			w.Write([]byte("Unsupported method"))
			return
		}

		data, header, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(501)
			w.Write([]byte(err.Error()))
			return
		}
		defer data.Close()

		filename := header.Filename
		extension := path.Ext(filename)

		sessionCookie, _ := r.Cookie("SESSION_KEY")
		fileID, err := s.db.AddFile(sessionCookie.Value, filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			w.WriteHeader(500)
			w.Write([]byte("Internal server error"))
			return
		}

		if err = s.saveFile(fileID, filename, data); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			w.WriteHeader(500)
			w.Write([]byte("Internal server error"))
			return
		}

		protocol := r.Header.Get("X-Forwarded-Proto")
		if protocol == "" {
			protocol = "http"
		}

		w.WriteHeader(200)
		w.Write([]byte(fmt.Sprintf("%s://%s/%s%s", protocol, r.Host, fileID, extension)))
	})
}

func (s *Server) handleFile() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint := r.URL.Path
		extension := path.Ext(endpoint)
		fileID := endpoint[1 : len(endpoint)-len(extension)]

		if r.Method == "GET" {
			filename, err := s.db.GetFile("", fileID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				w.WriteHeader(500)
				w.Write([]byte("Internal server error"))
				return
			}

			if filename == "" {
				w.WriteHeader(404)
				w.Write([]byte("File not found"))
				return
			}
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", filename))

			filepath := s.getFilePath(fileID, filename)
			http.ServeFile(w, r, filepath)
		} else if r.Method == "DELETE" {
			sessionCookie, _ := r.Cookie("SESSION_KEY")
			sessionKey := sessionCookie.Value

			filename, err := s.db.GetFile(sessionKey, fileID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				w.WriteHeader(500)
				w.Write([]byte("Internal server error"))
				return
			}

			if filename == "" {
				w.WriteHeader(404)
				w.Write([]byte("File not found"))
				return
			}

			if err = s.db.DeleteFile(sessionKey, fileID); err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				w.WriteHeader(500)
				w.Write([]byte("Internal server error"))
				return
			}

			if err = s.deleteFile(fileID, filename); err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				w.WriteHeader(500)
				w.Write([]byte("Internal server error"))
				return
			}

			w.WriteHeader(200)
		} else {
			w.WriteHeader(400)
			w.Write([]byte("Unsupported method"))
		}
	})
}

type UploadedFile struct {
	Id             string `json:"id"`
	Url            string `json:"url"`
	Name           string `json:"name"`
	Since          string `json:"since"`
	FileSizePretty string `json:"size-pretty"`
	FileSize       int64  `json:"size"`
}

func (s *Server) fillUploadedFiles(dbFiles []database.UploadedFile, url string) ([]UploadedFile, error) {
	var files []UploadedFile

	for _, dbFile := range dbFiles {
		filepath := s.getFilePath(dbFile.Id, dbFile.Name)
		filestats, err := os.Stat(filepath)
		if err != nil {
			return []UploadedFile{}, err
		}

		file := UploadedFile{
			Id:             dbFile.Id,
			Url:            url + "/" + dbFile.Id,
			Name:           dbFile.Name,
			Since:          dbFile.Since,
			FileSizePretty: prettyFileSize(filestats.Size()),
			FileSize:       filestats.Size(),
		}

		files = append(files, file)
	}

	return files, nil
}

func (s *Server) handleList() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(400)
			w.Write([]byte("Unsupported method"))
			return
		}

		sessionCookie, _ := r.Cookie("SESSION_KEY")
		dbFiles, err := s.db.ListFiles(sessionCookie.Value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			w.WriteHeader(500)
			w.Write([]byte("Internal server error"))
			return
		}

		files, err := s.fillUploadedFiles(dbFiles, s.getUrl(r))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			w.WriteHeader(500)
			w.Write([]byte("Internal server error"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
	})
}

func (s *Server) saveFile(fileID, filename string, data io.Reader) error {
	filepath := s.getFilePath(fileID, filename)

	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return err
	}

	written, err := io.Copy(file, data)
	if err != nil {
		return err
	}

	fmt.Printf("Written %d bytes into '%s'\n", written, filepath)

	return nil
}

func (s *Server) deleteFile(fileID, filename string) error {
	filepath := s.getFilePath(fileID, filename)

	if err := os.Remove(filepath); err != nil {
		return err
	}

	fmt.Printf("Deleted file '%s-%s'\n", filepath)

	return nil
}

func (s *Server) getFilePath(fileID, filename string) string {
	fullname := fmt.Sprintf("%s-%s", fileID, filename)
	return path.Join(s.root, fullname)
}

func prettyFileSize(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}

	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}
