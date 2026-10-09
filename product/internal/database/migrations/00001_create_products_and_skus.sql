-- +goose Up

CREATE TYPE product_status AS ENUM (
    'draft',
    'active',
    'archived'
);

CREATE TYPE product_skus_status AS ENUM (
    'active',
    'inactive'
);

CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status product_status NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT products_name_not_blank
        CHECK (length(btrim(name)) > 0)
);

CREATE TABLE product_skus (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL,
    code TEXT NOT NULL,
    price_cents BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'BRL',
    status product_skus_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT product_skus_product_fk
      FOREIGN KEY (product_id)
          REFERENCES products(id)
          ON DELETE RESTRICT,

    CONSTRAINT product_skus_code_unique
      UNIQUE (code),

    CONSTRAINT product_skus_code_not_blank
      CHECK (length(btrim(code)) > 0),

    CONSTRAINT product_skus_price_nonnegative
      CHECK (price_cents >= 0),

    CONSTRAINT product_skus_currency_format
      CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE INDEX product_skus_product_id_idx
    ON product_skus (product_id);

-- +goose Down
DROP TABLE product_skus;
DROP TABLE products;

DROP TYPE product_skus_status;
DROP TYPE product_status;
