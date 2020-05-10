package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"puush/database"
	"strings"
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
	s.muxer.Handle("/", s.log(s.handleFile()))

	return s, nil
}

func (s *Server) Serve() error {
	return http.ListenAndServe(":8080", s.muxer)
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

		protocol := ""
		for _, forwarded := range r.Header.Values("Forwarded") {
			for _, values := range strings.Split(forwarded, ";") {
				splitted := strings.Split(values, "=")
				if len(splitted) != 2 {
					continue
				}

				if splitted[0] == "proto" && (splitted[1] == "http" || splitted[1] == "https") {
					protocol = fmt.Sprintf("%s://", splitted[1])
				}
			}
		}

		w.WriteHeader(200)
		w.Write([]byte(fmt.Sprintf("%s%s/%s%s", protocol, r.Host, fileID, extension)))
	})
}

func (s *Server) handleFile() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(400)
			w.Write([]byte("Unsupported method"))
			return
		}

		endpoint := r.URL.Path
		extension := path.Ext(endpoint)
		fileID := endpoint[1 : len(endpoint)-len(extension)]
		filename, err := s.db.GetFile(fileID)
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

		fullname := fmt.Sprintf("%s-%s", fileID, filename)
		filepath := path.Join(s.root, fullname)
		http.ServeFile(w, r, filepath)
	})
}

func (s *Server) saveFile(fileID, filename string, data io.Reader) error {
	fullname := fmt.Sprintf("%s-%s", fileID, filename)
	filepath := path.Join(s.root, fullname)

	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return err
	}

	written, err := io.Copy(file, data)
	if err != nil {
		return err
	}

	fmt.Printf("Written %d bytes into '%s'\n", written, fullname)

	return nil
}
