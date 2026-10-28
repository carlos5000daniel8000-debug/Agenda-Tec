# Agenda Tec

Aplicação web para agendar visitas de assistência técnica de informática. O servidor é feito em Go e a interface usa HTML, CSS e JavaScript.

O formulário inclui serviços de informática e registra os pedidos em `data\visitas.csv`. O QR Code é gerado localmente e abre uma conversa no WhatsApp `+55 83 98835-4958`.

## Executar localmente

É necessário ter Go 1.22 ou mais recente instalado. No PowerShell, na pasta `agenda-tec`, execute:

```powershell
go run .
```

Abra `http://localhost:8080` no navegador. Para usar outra porta, defina a variável de ambiente `PORT` antes de iniciar o servidor.

As solicitações são gravadas em `data\visitas.csv`. Cada data e horário só pode ser solicitado uma vez. O formulário recebe pedidos; a equipe ainda precisa confirmar a disponibilidade da visita.

## Executar com Docker

Na pasta `agenda-tec`, construa e execute a imagem:

```powershell
docker build -t agenda-tec .
docker run --name agenda-tec -p 8080:8080 -v agenda-tec-dados:/app/data agenda-tec
```

Abra `http://localhost:8080`. O volume mantém os registros entre reinicializações do contêiner.
