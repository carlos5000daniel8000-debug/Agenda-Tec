package main

import (
	"bytes"
	"encoding/csv"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateVisitAcceptsComputerServices(t *testing.T) {
	now := time.Now()
	scheduled := now.Add(48 * time.Hour).Truncate(time.Minute)
	request := visit{
		name:    "Cliente de teste",
		phone:   "83999990000",
		address: "Rua de teste, 100",
		date:    scheduled.Format("2006-01-02"),
		time:    scheduled.Format("15:04"),
		service: "Conserto de computador/notebook",
	}

	if message := validateVisit(request, now); message != "" {
		t.Fatalf("visita válida rejeitada: %s", message)
	}
	request.service = "Instalação"
	if message := validateVisit(request, now); message == "" {
		t.Fatal("serviço antigo de refrigeração foi aceito")
	}
}

func TestHandleQRCodeReturnsPNG(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/qrcode.png", nil)
	response := httptest.NewRecorder()

	handleQRCode(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("esperado status %d, recebido %d", http.StatusOK, response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type inesperado: %q", got)
	}
	image, err := png.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("resposta não contém PNG válido: %v", err)
	}
	if image.Bounds().Dx() != 256 || image.Bounds().Dy() != 256 {
		t.Fatalf("dimensões inesperadas para o QR Code: %v", image.Bounds())
	}
}

func TestHandleVisitPersistsAndRejectsDuplicateTime(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "visitas.csv")
	t.Setenv("VISITS_FILE", filePath)
	scheduled := time.Now().Add(72 * time.Hour).Truncate(time.Minute)
	form := url.Values{
		"nome":     {"Cliente de teste"},
		"telefone": {"83999990000"},
		"endereco": {"Rua de teste, 100"},
		"data":     {scheduled.Format("2006-01-02")},
		"horario":  {scheduled.Format("15:04")},
		"servico":  {"Rede e Wi-Fi"},
		"detalhes": {"Verificar conexão"},
	}

	first := postVisit(form)
	if first.Code != http.StatusCreated {
		t.Fatalf("esperado status %d, recebido %d: %s", http.StatusCreated, first.Code, first.Body)
	}
	if !strings.Contains(first.Body.String(), "Pedido recebido") {
		t.Fatalf("resposta de confirmação inesperada: %s", first.Body)
	}

	duplicate := postVisit(form)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("esperado status %d para horário repetido, recebido %d", http.StatusConflict, duplicate.Code)
	}

	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("não foi criado o arquivo de visitas: %v", err)
	}
	defer file.Close()

	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("não foi possível ler o arquivo de visitas: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("esperado um registro, recebidos %d", len(records))
	}
}

func postVisit(form url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/visitas", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handleVisit(response, request)
	return response
}
