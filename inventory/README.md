# Inventory

Microsserviço responsável pelo saldo de estoque e pelo ciclo de vida das reservas de SKU.

## Regras implementadas

- A disponibilidade é calculada como `quantity_on_hand - quantity_reserved`.
- A reserva só é criada quando há saldo disponível e a criação é idempotente por `reservation_id`.
- Reutilizar o mesmo `reservation_id` com outro pedido, SKU ou quantidade produz conflito.
- Uma reserva começa como `active` e termina como `confirmed` ou `released`.
- Confirmar consome o estoque: reduz `quantity_on_hand` e `quantity_reserved` na mesma transação.
- Liberar devolve a disponibilidade: reduz apenas `quantity_reserved`.
- Repetir a mesma transição terminal é idempotente. Transições terminais incompatíveis produzem conflito.
- Reservas ativas expiram após o TTL. Um worker libera um lote por intervalo e continua processando os demais registros mesmo quando um item do lote falha.
- Locks de linha e updates condicionais no PostgreSQL serializam transições concorrentes e impedem saldo negativo.

## API HTTP

| Método | Rota | Função |
| --- | --- | --- |
| `POST` | `/inventory` | Cadastra o saldo inicial de um SKU |
| `GET` | `/inventory/{sku_id}` | Consulta saldo físico, reservado e disponível |
| `POST` | `/inventory/reservations` | Cria uma reserva idempotente |
| `POST` | `/inventory/reservations/{id}/confirm` | Confirma a reserva e consome o estoque |
| `POST` | `/inventory/reservations/{id}/release` | Libera a reserva |

Exemplo de criação de reserva:

```json
{
  "reservation_id": "11111111-1111-1111-1111-111111111111",
  "order_id": "22222222-2222-2222-2222-222222222222",
  "sku_id": "33333333-3333-3333-3333-333333333333",
  "quantity": 2
}
```

## Configuração

O processo lê estas variáveis:

- `DATABASE_URI`; quando ausente, usa `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_HOST`, `POSTGRES_PORT` e `POSTGRES_DB`.
- `INVENTORY_RESERVATION_TTL`, padrão `15m` e mínimo `1s`.
- `INVENTORY_EXPIRATION_INTERVAL`, padrão `30s`.
- `INVENTORY_EXPIRATION_BATCH_SIZE`, padrão `100`.
- `INVENTORY_HTTP_ADDRESS`, padrão `:9998`.

O arquivo `.env` é carregado quando existe. Em ambientes implantados, as variáveis podem ser injetadas diretamente no processo.

## Execução local

Na raiz do repositório:

```bash
docker compose -f docker/docker-compose.yaml up -d inventory_db inventory_test_db
```

Dentro de `inventory`, aplique as migrations no banco local e inicie a API:

```bash
make migrate
make run
```

Os comandos de qualidade e teste são:

```bash
make test
make test-race
make vet
```

`make test` aplica automaticamente as migrations somente no banco cujo nome termina em `_test` e executa os testes de unidade, HTTP, integração e concorrência.

## Encerramento

`SIGINT` e `SIGTERM` cancelam o worker, encerram o servidor HTTP com prazo de dez segundos e fecham o pool PostgreSQL.
