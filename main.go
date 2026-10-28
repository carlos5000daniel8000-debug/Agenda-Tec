package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	qrcode "github.com/skip2/go-qrcode"
)

const whatsappURL = "https://wa.me/5583988354958"

var (
	visitsMu = &sync.Mutex{}
	services = map[string]bool{
		"Conserto de computador/notebook": true,
		"Formatação e reinstalação":       true,
		"Instalação de programas":         true,
		"Remoção de vírus":                true,
		"Rede e Wi-Fi":                    true,
		"Manutenção preventiva":           true,
		"Outro":                           true,
	}
)

type visit struct {
	name    string
	phone   string
	address string
	date    string
	time    string
	service string
	details string
}

func main() {
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		log.Fatalf("porta inválida %q: %v", port, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handlePage)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/qrcode.png", handleQRCode)
	mux.HandleFunc("/api/visitas", handleVisit)

	addr := net.JoinHostPort(host, port)
	log.Printf("Agenda Tec disponível em http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("servidor Agenda Tec encerrado: %v", err)
	}
}

func handlePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Método não permitido.", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}

	page, err := os.ReadFile(filepath.Join("public", "index.html"))
	if err != nil {
		log.Printf("não foi possível ler a página inicial: %v", err)
		http.Error(w, "Página não disponível.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(page)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Método não permitido.", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}

func handleQRCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Método não permitido.", http.StatusMethodNotAllowed)
		return
	}

	image, err := qrcode.Encode(whatsappURL, qrcode.Medium, 256)
	if err != nil {
		log.Printf("não foi possível gerar o QR Code do WhatsApp: %v", err)
		http.Error(w, "QR Code não disponível.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(image)
}

func handleVisit(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/visitas" {
		writeJSON(w, http.StatusNotFound, map[string]string{"erro": "Rota não encontrada."})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"erro": "Use o formulário para enviar uma visita.",
		})
		return
	}

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"erro": "Formato de envio não permitido.",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16_384)
	if err := r.ParseForm(); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
				"erro": "O formulário enviado é muito grande.",
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"erro": "Não foi possível ler os dados enviados.",
		})
		return
	}

	fields := []string{"nome", "telefone", "endereco", "data", "horario", "servico", "detalhes"}
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		formValues := r.PostForm[field]
		if len(formValues) > 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"erro": "Não foi possível ler os dados enviados.",
			})
			return
		}
		if len(formValues) == 1 {
			values[field] = clean(formValues[0])
		}
	}

	request := visit{
		name:    values["nome"],
		phone:   values["telefone"],
		address: values["endereco"],
		date:    values["data"],
		time:    values["horario"],
		service: values["servico"],
		details: values["detalhes"],
	}
	if message := validateVisit(request, time.Now()); message != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"erro": message})
		return
	}

	visitsMu.Lock()
	defer visitsMu.Unlock()

	filePath := os.Getenv("VISITS_FILE")
	if filePath == "" {
		filePath = filepath.Join("data", "visitas.csv")
	}
	conflict, err := hasConflictingVisit(filePath, request.date, request.time)
	if err != nil {
		log.Printf("não foi possível verificar horários: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"erro": "Não foi possível registrar a visita. Tente novamente mais tarde.",
		})
		return
	}
	if conflict {
		writeJSON(w, http.StatusConflict, map[string]string{
			"erro": "Esse horário já foi solicitado. Escolha outro horário.",
		})
		return
	}
	if err := saveVisit(filePath, request); err != nil {
		log.Printf("não foi possível salvar a visita: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"erro": "Não foi possível registrar a visita. Tente novamente mais tarde.",
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"mensagem": "Pedido recebido! Entraremos em contato para confirmar a visita.",
	})
}

func validateVisit(v visit, now time.Time) string {
	if runeCount(v.name) < 2 || runeCount(v.name) > 100 {
		return "Informe seu nome (de 2 a 100 caracteres)."
	}
	digits := 0
	for _, char := range v.phone {
		if char >= '0' && char <= '9' {
			digits++
		}
	}
	if digits < 10 || digits > 15 || runeCount(v.phone) > 30 {
		return "Informe um telefone válido com DDD."
	}
	if runeCount(v.address) < 5 || runeCount(v.address) > 200 {
		return "Informe o endereço da visita (de 5 a 200 caracteres)."
	}
	if !services[v.service] {
		return "Selecione um tipo de serviço válido."
	}
	if runeCount(v.details) > 500 {
		return "As observações devem ter no máximo 500 caracteres."
	}

	date, err := time.ParseInLocation("2006-01-02", v.date, now.Location())
	if err != nil {
		return "Informe uma data e um horário válidos."
	}
	visitTime, err := time.Parse("15:04", v.time)
	if err != nil {
		return "Informe uma data e um horário válidos."
	}
	scheduled := time.Date(
		date.Year(), date.Month(), date.Day(),
		visitTime.Hour(), visitTime.Minute(), 0, 0, now.Location())
	if scheduled.Before(now) {
		return "Escolha uma data e um horário futuros."
	}
	return ""
}

func clean(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}
		return r
	}, value)
	return strings.TrimSpace(value)
}

func runeCount(value string) int {
	return utf8.RuneCountInString(value)
}

func hasConflictingVisit(filePath, date, visitTime string) (bool, error) {
	file, err := os.Open(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(record) >= 2 && record[0] == date && record[1] == visitTime {
			return true, nil
		}
	}
}

func saveVisit(filePath string, v visit) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{
		v.date, v.time, v.name, v.phone, v.address, v.service, v.details,
	}); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

func writeJSON(w http.ResponseWriter, status int, value map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("não foi possível escrever resposta JSON: %v", err)
	}
}
