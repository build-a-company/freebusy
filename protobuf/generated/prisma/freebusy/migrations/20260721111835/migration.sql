-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "booking";

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "channel";

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "common";

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "identity";

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "organisation";

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "promocode";

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "property";

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "schedule";

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "shared";

-- CreateEnum
CREATE TYPE "booking"."booking_state" AS ENUM ('PENDING_HOLD', 'CONFIRMED', 'CANCELLED', 'EXPIRED', 'COMPLETED', 'NO_SHOW');

-- CreateEnum
CREATE TYPE "booking"."cancel_reason" AS ENUM ('REQUESTED_BY_CUSTOMER', 'REQUESTED_BY_OPERATOR', 'PAYMENT_FAILED', 'NO_SHOW', 'OTHER');

-- CreateEnum
CREATE TYPE "channel"."channel_type" AS ENUM ('AGODA', 'BOOKING_COM', 'EXPEDIA', 'AIRBNB', 'MAKEMYTRIP', 'GOIBIBO', 'GDS', 'DIRECT');

-- CreateEnum
CREATE TYPE "channel"."channel_state" AS ENUM ('CONNECTED', 'DISABLED', 'ERROR');

-- CreateEnum
CREATE TYPE "channel"."mapping_state" AS ENUM ('MAPPED', 'UNMAPPED');

-- CreateEnum
CREATE TYPE "identity"."gender" AS ENUM ('MALE', 'FEMALE', 'OTHER', 'UNDISCLOSED');

-- CreateEnum
CREATE TYPE "identity"."age_group" AS ENUM ('ADULT', 'CHILD', 'INFANT');

-- CreateEnum
CREATE TYPE "identity"."id_document_type" AS ENUM ('PASSPORT', 'NATIONAL_ID', 'DRIVING_LICENSE', 'AADHAAR', 'VOTER_ID', 'OTHER');

-- CreateEnum
CREATE TYPE "identity"."smoking_preference" AS ENUM ('NON_SMOKING', 'SMOKING');

-- CreateEnum
CREATE TYPE "identity"."bed_preference" AS ENUM ('NO_PREFERENCE', 'KING', 'QUEEN', 'TWIN', 'SINGLE');

-- CreateEnum
CREATE TYPE "organisation"."organisation_state" AS ENUM ('ACTIVE', 'SUSPENDED');

-- CreateEnum
CREATE TYPE "organisation"."organisation_role" AS ENUM ('OWNER', 'ADMIN', 'MEMBER', 'VIEWER');

-- CreateEnum
CREATE TYPE "organisation"."member_state" AS ENUM ('INVITED', 'ACTIVE', 'SUSPENDED');

-- CreateEnum
CREATE TYPE "promocode"."promo_code_state" AS ENUM ('ACTIVE', 'DISABLED', 'EXPIRED');

-- CreateEnum
CREATE TYPE "promocode"."discount_amount_case" AS ENUM ('PERCENT_OFF', 'AMOUNT_OFF');

-- CreateEnum
CREATE TYPE "property"."property_state" AS ENUM ('ACTIVE', 'ARCHIVED');

-- CreateEnum
CREATE TYPE "property"."unit_type" AS ENUM ('PROVIDER', 'ROOM', 'EQUIPMENT', 'LODGING', 'SPACE');

-- CreateEnum
CREATE TYPE "property"."pricing_unit" AS ENUM ('PER_BOOKING', 'PER_NIGHT', 'PER_PERSON');

-- CreateEnum
CREATE TYPE "property"."unit_state" AS ENUM ('ACTIVE', 'ARCHIVED');

-- CreateEnum
CREATE TYPE "property"."licence_target" AS ENUM ('PROPERTY', 'UNIT');

-- CreateEnum
CREATE TYPE "property"."licence_type" AS ENUM ('TRADE', 'FIRE_SAFETY', 'LIQUOR', 'FOOD_SAFETY', 'TOURISM', 'HEALTH', 'OTHER');

-- CreateEnum
CREATE TYPE "property"."licence_state" AS ENUM ('ACTIVE', 'ARCHIVED');

-- CreateEnum
CREATE TYPE "schedule"."exception_kind" AS ENUM ('CLOSURE', 'EXTRA_HOURS');

-- CreateEnum
CREATE TYPE "schedule"."availability_exception_span_case" AS ENUM ('WINDOW', 'DATE_RANGE');

-- CreateEnum
CREATE TYPE "property"."booking_mode" AS ENUM ('TIME_SLOT', 'NIGHTLY');

-- CreateEnum
CREATE TYPE "property"."media_type" AS ENUM ('IMAGE', 'VIDEO', 'DOCUMENT', 'FLOORPLAN', 'VIRTUAL_TOUR');

-- CreateEnum
CREATE TYPE "shared"."idempotency_state" AS ENUM ('IN_FLIGHT', 'DONE');

-- CreateEnum
CREATE TYPE "shared"."type" AS ENUM ('BASE', 'FEE', 'TAX', 'DISCOUNT');

-- CreateTable
CREATE TABLE "booking"."resource" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "unit" TEXT NOT NULL,
    "customer" TEXT,
    "units" INTEGER,
    "assigned_unit" TEXT,
    "state" "booking"."booking_state" DEFAULT 'PENDING_HOLD',
    "hold_expire_time" TIMESTAMPTZ(6),
    "promo_code" TEXT,
    "notes" TEXT,
    "attributes" JSONB,
    "cancel_reason" "booking"."cancel_reason",
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "confirm_time" TIMESTAMPTZ(6),
    "cancel_time" TIMESTAMPTZ(6),
    "refund_percent" INTEGER,
    "hold_ttl" TEXT,
    "etag" TEXT,
    "contact_id" TEXT,
    "occupancy_id" TEXT,
    "window_id" TEXT NOT NULL,
    "price_id" TEXT,
    "discount_id" TEXT,
    "total_id" TEXT,
    "refund_amount_id" TEXT,

    CONSTRAINT "resource_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "booking"."occupancies" (
    "id" TEXT NOT NULL,
    "adults" INTEGER,
    "children" INTEGER,
    "infants" INTEGER,

    CONSTRAINT "occupancies_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "channel"."resource" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "property" TEXT NOT NULL,
    "type" "channel"."channel_type" NOT NULL DEFAULT 'AGODA',
    "display_name" TEXT,
    "external_property_id" TEXT,
    "credential_ref" TEXT,
    "state" "channel"."channel_state",
    "last_sync_time" TIMESTAMPTZ(6),
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,

    CONSTRAINT "resource_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "channel"."unit_mappings" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "unit" TEXT NOT NULL,
    "external_room_type_id" TEXT NOT NULL,
    "external_rate_plan_id" TEXT,
    "state" "channel"."mapping_state",
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,
    "channel_id" TEXT NOT NULL,

    CONSTRAINT "unit_mappings_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "channel"."sync_statuses" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "state" "channel"."channel_state",
    "last_sync_time" TIMESTAMPTZ(6),
    "pending_count" INTEGER,
    "failed_count" INTEGER,
    "last_error" TEXT,

    CONSTRAINT "sync_statuses_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "identity"."guests" (
    "id" TEXT NOT NULL,
    "display_name" TEXT NOT NULL,
    "primary" BOOLEAN,
    "gender" "identity"."gender",
    "birth_date" DATE,
    "age_group" "identity"."age_group",
    "nationality" TEXT,
    "email" TEXT,
    "phone_number" TEXT,
    "booking_id" TEXT NOT NULL,
    "id_document_id" TEXT,
    "permanent_address_id" TEXT,
    "local_address_id" TEXT,
    "foreigner_id" TEXT,
    "preferences_id" TEXT,

    CONSTRAINT "guests_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "identity"."id_documents" (
    "id" TEXT NOT NULL,
    "type" "identity"."id_document_type" NOT NULL DEFAULT 'PASSPORT',
    "number" TEXT NOT NULL,
    "issuing_country" TEXT,
    "issue_place" TEXT,
    "issue_date" DATE,
    "expiry_date" DATE,
    "attachment_id" TEXT,

    CONSTRAINT "id_documents_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "identity"."foreigner_details" (
    "id" TEXT NOT NULL,
    "visa_number" TEXT,
    "visa_type" TEXT,
    "visa_issue_place" TEXT,
    "visa_issue_date" DATE,
    "visa_expiry_date" DATE,
    "arrival_date" DATE,
    "entry_port" TEXT,
    "origin" TEXT,
    "next_destination" TEXT,
    "visit_purpose" TEXT,

    CONSTRAINT "foreigner_details_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "identity"."guest_preferences" (
    "id" TEXT NOT NULL,
    "smoking" "identity"."smoking_preference",
    "bed" "identity"."bed_preference",
    "dietary" TEXT[],
    "accessibility" TEXT[],
    "floor_preference" INTEGER,
    "loyalty_number" TEXT,
    "special_requests" TEXT[],
    "notes" TEXT,

    CONSTRAINT "guest_preferences_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "identity"."users" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "email" TEXT,
    "display_name" TEXT,
    "avatar_url" TEXT,
    "locale" TEXT,
    "time_zone" TEXT,
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,

    CONSTRAINT "users_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "organisation"."resource" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "display_name" TEXT NOT NULL,
    "slug" TEXT,
    "billing_email" TEXT,
    "state" "organisation"."organisation_state" DEFAULT 'ACTIVE',
    "settings" JSONB,
    "member_count" BIGINT,
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,

    CONSTRAINT "resource_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "organisation"."members" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "user" TEXT,
    "email" TEXT NOT NULL,
    "display_name" TEXT,
    "role" "organisation"."organisation_role" NOT NULL DEFAULT 'OWNER',
    "state" "organisation"."member_state" DEFAULT 'INVITED',
    "inviter" TEXT,
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,
    "organisation_id" TEXT NOT NULL,

    CONSTRAINT "members_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "promocode"."resource" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "code" TEXT NOT NULL,
    "display_name" TEXT,
    "description" TEXT,
    "redemption_count" BIGINT,
    "state" "promocode"."promo_code_state",
    "disabled" BOOLEAN,
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,
    "discount_id" TEXT NOT NULL,
    "window_id" TEXT,
    "limits_id" TEXT,
    "scope_id" TEXT,

    CONSTRAINT "resource_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "promocode"."redemptions" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "customer" TEXT NOT NULL,
    "booking" TEXT NOT NULL,
    "redeemed_time" TIMESTAMPTZ(6),
    "promo_code_id" TEXT NOT NULL,
    "amount_applied_id" TEXT,

    CONSTRAINT "redemptions_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "promocode"."discounts" (
    "id" TEXT NOT NULL,
    "percent_off" INTEGER,
    "amount_case" "promocode"."discount_amount_case",
    "amount_off_id" TEXT,

    CONSTRAINT "discounts_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "promocode"."redemption_windows" (
    "id" TEXT NOT NULL,
    "start_time" TIMESTAMPTZ(6),
    "end_time" TIMESTAMPTZ(6),

    CONSTRAINT "redemption_windows_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "promocode"."usage_limits" (
    "id" TEXT NOT NULL,
    "max_redemptions" BIGINT,
    "per_customer_limit" INTEGER,

    CONSTRAINT "usage_limits_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "promocode"."scopes" (
    "id" TEXT NOT NULL,
    "min_subtotal_id" TEXT,

    CONSTRAINT "scopes_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "promocode"."scope_applicable_properties" (
    "id" TEXT NOT NULL,
    "scope_id" TEXT NOT NULL,
    "property_id" TEXT NOT NULL,

    CONSTRAINT "scope_applicable_properties_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "promocode"."scope_applicable_units" (
    "id" TEXT NOT NULL,
    "scope_id" TEXT NOT NULL,
    "unit_id" TEXT NOT NULL,
    "unit_name" TEXT NOT NULL,

    CONSTRAINT "scope_applicable_units_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."properties" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "organisation" TEXT NOT NULL,
    "display_name" TEXT NOT NULL,
    "description" TEXT,
    "time_zone" TEXT NOT NULL,
    "tags" TEXT[],
    "attributes" JSONB,
    "state" "property"."property_state" DEFAULT 'ACTIVE',
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,
    "address_id" TEXT,
    "policy_id" TEXT,

    CONSTRAINT "properties_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."units" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "display_name" TEXT NOT NULL,
    "description" TEXT,
    "type" "property"."unit_type" NOT NULL DEFAULT 'PROVIDER',
    "booking_mode" "property"."booking_mode" NOT NULL DEFAULT 'TIME_SLOT',
    "capacity" INTEGER,
    "max_occupancy" INTEGER,
    "time_zone" TEXT NOT NULL,
    "pricing_unit" "property"."pricing_unit",
    "duration" TEXT,
    "tags" TEXT[],
    "attributes" JSONB,
    "state" "property"."unit_state" DEFAULT 'ACTIVE',
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,
    "property_id" TEXT NOT NULL,
    "price_id" TEXT,

    CONSTRAINT "units_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."licences" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "target" "property"."licence_target",
    "unit" TEXT,
    "type" "property"."licence_type" NOT NULL DEFAULT 'TRADE',
    "licence_number" TEXT,
    "issuing_authority" TEXT,
    "issue_date" DATE,
    "expiry_date" DATE,
    "notes" TEXT,
    "state" "property"."licence_state" DEFAULT 'ACTIVE',
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,
    "etag" TEXT,
    "property_id" TEXT NOT NULL,
    "attachment_id" TEXT,

    CONSTRAINT "licences_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."medias" (
    "id" TEXT NOT NULL,
    "uri" TEXT NOT NULL,
    "type" "property"."media_type" NOT NULL DEFAULT 'IMAGE',
    "title" TEXT,
    "description" TEXT,
    "mime_type" TEXT,
    "sort_order" INTEGER,
    "primary" BOOLEAN,
    "property_id" TEXT NOT NULL,

    CONSTRAINT "medias_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."policies" (
    "id" TEXT NOT NULL,
    "checkin_time" TIME(6),
    "checkout_time" TIME(6),
    "house_rules" TEXT[],
    "notes" TEXT,

    CONSTRAINT "policies_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."rate_overrides" (
    "id" TEXT NOT NULL,
    "weekdays" TEXT,
    "unit_id" TEXT NOT NULL,
    "date_range_id" TEXT,
    "price_id" TEXT NOT NULL,

    CONSTRAINT "rate_overrides_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."los_discounts" (
    "id" TEXT NOT NULL,
    "min_nights" INTEGER NOT NULL,
    "percent_off" INTEGER,
    "unit_id" TEXT NOT NULL,
    "amount_off_id" TEXT,

    CONSTRAINT "los_discounts_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."fees" (
    "id" TEXT NOT NULL,
    "code" TEXT NOT NULL,
    "display_name" TEXT,
    "percent" INTEGER,
    "pricing_unit" "property"."pricing_unit",
    "taxable" BOOLEAN,
    "unit_id" TEXT NOT NULL,
    "amount_id" TEXT,

    CONSTRAINT "fees_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."taxes" (
    "id" TEXT NOT NULL,
    "code" TEXT NOT NULL,
    "display_name" TEXT,
    "percent" DOUBLE PRECISION NOT NULL,
    "unit_id" TEXT NOT NULL,

    CONSTRAINT "taxes_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."unit_medias" (
    "id" TEXT NOT NULL,
    "uri" TEXT NOT NULL,
    "type" "property"."media_type" NOT NULL DEFAULT 'IMAGE',
    "title" TEXT,
    "description" TEXT,
    "mime_type" TEXT,
    "sort_order" INTEGER,
    "primary" BOOLEAN,
    "unit_id" TEXT NOT NULL,

    CONSTRAINT "unit_medias_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."units_link" (
    "id" TEXT NOT NULL,
    "property_id" TEXT NOT NULL,
    "unit_id" TEXT NOT NULL,
    "unit_name" TEXT NOT NULL,

    CONSTRAINT "units_link_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "property"."unit_applicable_promo_codes" (
    "id" TEXT NOT NULL,
    "unit_id" TEXT NOT NULL,
    "promo_code_id" TEXT NOT NULL,

    CONSTRAINT "unit_applicable_promo_codes_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "schedule"."availability_exceptions" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "kind" "schedule"."exception_kind" NOT NULL DEFAULT 'CLOSURE',
    "reason" TEXT,
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "span_case" "schedule"."availability_exception_span_case",
    "property_id" TEXT NOT NULL,
    "unit_id" TEXT NOT NULL,
    "window_id" TEXT,
    "date_range_id" TEXT,

    CONSTRAINT "availability_exceptions_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "schedule"."resource" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "etag" TEXT,
    "property_id" TEXT NOT NULL,
    "buffers_id" TEXT,
    "stay_constraints_id" TEXT,
    "cancellation_policy_id" TEXT,

    CONSTRAINT "resource_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "schedule"."recurring_rules" (
    "id" TEXT NOT NULL,
    "rrule" TEXT NOT NULL,
    "opens" TEXT,
    "closes" TEXT,
    "schedule_id" TEXT NOT NULL,

    CONSTRAINT "recurring_rules_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "schedule"."buffer_settings" (
    "id" TEXT NOT NULL,
    "start_delta" TEXT,
    "end_delta" TEXT,
    "min_notice" TEXT,
    "max_advance" TEXT,
    "gap" TEXT,

    CONSTRAINT "buffer_settings_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "schedule"."stay_constraints" (
    "id" TEXT NOT NULL,
    "min_nights" INTEGER,
    "max_nights" INTEGER,
    "checkin_weekdays" TEXT,
    "checkout_weekdays" TEXT,
    "advance_min_days" INTEGER,
    "advance_max_days" INTEGER,

    CONSTRAINT "stay_constraints_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "schedule"."cancellation_policies" (
    "id" TEXT NOT NULL,

    CONSTRAINT "cancellation_policies_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "schedule"."refund_tiers" (
    "id" TEXT NOT NULL,
    "cutoff" TEXT NOT NULL,
    "refund_percent" INTEGER NOT NULL,
    "cancellation_policy_id" TEXT NOT NULL,

    CONSTRAINT "refund_tiers_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "schedule"."exceptions" (
    "id" TEXT NOT NULL,
    "schedule_id" TEXT NOT NULL,
    "availability_exception_id" TEXT NOT NULL,
    "availability_exception_name" TEXT NOT NULL,

    CONSTRAINT "exceptions_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "shared"."idempotency_keys" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "method" TEXT NOT NULL,
    "request_id" TEXT NOT NULL,
    "state" "shared"."idempotency_state" NOT NULL DEFAULT 'IN_FLIGHT',
    "response" TEXT,
    "create_time" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "update_time" TIMESTAMPTZ(6) NOT NULL,

    CONSTRAINT "idempotency_keys_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "shared"."contacts" (
    "id" TEXT NOT NULL,
    "display_name" TEXT,
    "email" TEXT,
    "phone_number" TEXT,

    CONSTRAINT "contacts_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "shared"."time_windows" (
    "id" TEXT NOT NULL,
    "start_time" TIMESTAMPTZ(6) NOT NULL,
    "end_time" TIMESTAMPTZ(6) NOT NULL,

    CONSTRAINT "time_windows_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "shared"."price_components" (
    "id" TEXT NOT NULL,
    "type" "shared"."type",
    "code" TEXT,
    "display_name" TEXT,
    "booking_id" TEXT NOT NULL,
    "amount_id" TEXT,

    CONSTRAINT "price_components_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "shared"."attachments" (
    "id" TEXT NOT NULL,
    "filename" TEXT,
    "mime_type" TEXT,
    "size_bytes" BIGINT,
    "content" BYTEA,
    "uri" TEXT,
    "upload_time" TIMESTAMPTZ(6),

    CONSTRAINT "attachments_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "shared"."date_ranges" (
    "id" TEXT NOT NULL,
    "start_date" DATE NOT NULL,
    "end_date" DATE NOT NULL,

    CONSTRAINT "date_ranges_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "common"."moneys" (
    "id" TEXT NOT NULL,
    "currency_code" TEXT,
    "units" BIGINT,
    "nanos" INTEGER,

    CONSTRAINT "moneys_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "common"."postal_address" (
    "id" TEXT NOT NULL,
    "revision" INTEGER,
    "region_code" TEXT,
    "language_code" TEXT,
    "postal_code" TEXT,
    "sorting_code" TEXT,
    "administrative_area" TEXT,
    "locality" TEXT,
    "sublocality" TEXT,
    "address_lines" TEXT[],
    "recipients" TEXT[],
    "organization" TEXT,

    CONSTRAINT "postal_address_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "resource_name_key" ON "booking"."resource"("name");

-- CreateIndex
CREATE INDEX "resource_unit_idx" ON "booking"."resource"("unit");

-- CreateIndex
CREATE INDEX "resource_customer_idx" ON "booking"."resource"("customer");

-- CreateIndex
CREATE INDEX "resource_promo_code_idx" ON "booking"."resource"("promo_code");

-- CreateIndex
CREATE INDEX "resource_contact_id_idx" ON "booking"."resource"("contact_id");

-- CreateIndex
CREATE INDEX "resource_occupancy_id_idx" ON "booking"."resource"("occupancy_id");

-- CreateIndex
CREATE INDEX "resource_window_id_idx" ON "booking"."resource"("window_id");

-- CreateIndex
CREATE INDEX "resource_price_id_idx" ON "booking"."resource"("price_id");

-- CreateIndex
CREATE INDEX "resource_discount_id_idx" ON "booking"."resource"("discount_id");

-- CreateIndex
CREATE INDEX "resource_total_id_idx" ON "booking"."resource"("total_id");

-- CreateIndex
CREATE INDEX "resource_refund_amount_id_idx" ON "booking"."resource"("refund_amount_id");

-- CreateIndex
CREATE UNIQUE INDEX "resource_name_key" ON "channel"."resource"("name");

-- CreateIndex
CREATE INDEX "resource_property_idx" ON "channel"."resource"("property");

-- CreateIndex
CREATE UNIQUE INDEX "unit_mappings_name_key" ON "channel"."unit_mappings"("name");

-- CreateIndex
CREATE INDEX "unit_mappings_unit_idx" ON "channel"."unit_mappings"("unit");

-- CreateIndex
CREATE INDEX "unit_mappings_channel_id_idx" ON "channel"."unit_mappings"("channel_id");

-- CreateIndex
CREATE UNIQUE INDEX "sync_statuses_name_key" ON "channel"."sync_statuses"("name");

-- CreateIndex
CREATE INDEX "guests_booking_id_idx" ON "identity"."guests"("booking_id");

-- CreateIndex
CREATE INDEX "guests_id_document_id_idx" ON "identity"."guests"("id_document_id");

-- CreateIndex
CREATE INDEX "guests_permanent_address_id_idx" ON "identity"."guests"("permanent_address_id");

-- CreateIndex
CREATE INDEX "guests_local_address_id_idx" ON "identity"."guests"("local_address_id");

-- CreateIndex
CREATE INDEX "guests_foreigner_id_idx" ON "identity"."guests"("foreigner_id");

-- CreateIndex
CREATE INDEX "guests_preferences_id_idx" ON "identity"."guests"("preferences_id");

-- CreateIndex
CREATE INDEX "id_documents_attachment_id_idx" ON "identity"."id_documents"("attachment_id");

-- CreateIndex
CREATE UNIQUE INDEX "users_name_key" ON "identity"."users"("name");

-- CreateIndex
CREATE UNIQUE INDEX "resource_name_key" ON "organisation"."resource"("name");

-- CreateIndex
CREATE UNIQUE INDEX "members_name_key" ON "organisation"."members"("name");

-- CreateIndex
CREATE INDEX "members_user_idx" ON "organisation"."members"("user");

-- CreateIndex
CREATE INDEX "members_inviter_idx" ON "organisation"."members"("inviter");

-- CreateIndex
CREATE INDEX "members_organisation_id_idx" ON "organisation"."members"("organisation_id");

-- CreateIndex
CREATE UNIQUE INDEX "resource_name_key" ON "promocode"."resource"("name");

-- CreateIndex
CREATE UNIQUE INDEX "resource_code_key" ON "promocode"."resource"("code");

-- CreateIndex
CREATE INDEX "resource_discount_id_idx" ON "promocode"."resource"("discount_id");

-- CreateIndex
CREATE INDEX "resource_window_id_idx" ON "promocode"."resource"("window_id");

-- CreateIndex
CREATE INDEX "resource_limits_id_idx" ON "promocode"."resource"("limits_id");

-- CreateIndex
CREATE INDEX "resource_scope_id_idx" ON "promocode"."resource"("scope_id");

-- CreateIndex
CREATE UNIQUE INDEX "redemptions_name_key" ON "promocode"."redemptions"("name");

-- CreateIndex
CREATE INDEX "redemptions_customer_idx" ON "promocode"."redemptions"("customer");

-- CreateIndex
CREATE INDEX "redemptions_booking_idx" ON "promocode"."redemptions"("booking");

-- CreateIndex
CREATE INDEX "redemptions_promo_code_id_idx" ON "promocode"."redemptions"("promo_code_id");

-- CreateIndex
CREATE INDEX "redemptions_amount_applied_id_idx" ON "promocode"."redemptions"("amount_applied_id");

-- CreateIndex
CREATE INDEX "discounts_amount_off_id_idx" ON "promocode"."discounts"("amount_off_id");

-- CreateIndex
CREATE INDEX "scopes_min_subtotal_id_idx" ON "promocode"."scopes"("min_subtotal_id");

-- CreateIndex
CREATE INDEX "scope_applicable_properties_property_id_idx" ON "promocode"."scope_applicable_properties"("property_id");

-- CreateIndex
CREATE UNIQUE INDEX "scope_applicable_properties_scope_id_property_id_key" ON "promocode"."scope_applicable_properties"("scope_id", "property_id");

-- CreateIndex
CREATE INDEX "scope_applicable_units_unit_id_idx" ON "promocode"."scope_applicable_units"("unit_id");

-- CreateIndex
CREATE UNIQUE INDEX "scope_applicable_units_scope_id_unit_id_key" ON "promocode"."scope_applicable_units"("scope_id", "unit_id");

-- CreateIndex
CREATE UNIQUE INDEX "properties_name_key" ON "property"."properties"("name");

-- CreateIndex
CREATE INDEX "properties_organisation_idx" ON "property"."properties"("organisation");

-- CreateIndex
CREATE INDEX "properties_address_id_idx" ON "property"."properties"("address_id");

-- CreateIndex
CREATE INDEX "properties_policy_id_idx" ON "property"."properties"("policy_id");

-- CreateIndex
CREATE UNIQUE INDEX "units_name_key" ON "property"."units"("name");

-- CreateIndex
CREATE INDEX "units_property_id_idx" ON "property"."units"("property_id");

-- CreateIndex
CREATE INDEX "units_price_id_idx" ON "property"."units"("price_id");

-- CreateIndex
CREATE UNIQUE INDEX "licences_name_key" ON "property"."licences"("name");

-- CreateIndex
CREATE INDEX "licences_unit_idx" ON "property"."licences"("unit");

-- CreateIndex
CREATE INDEX "licences_property_id_idx" ON "property"."licences"("property_id");

-- CreateIndex
CREATE INDEX "licences_attachment_id_idx" ON "property"."licences"("attachment_id");

-- CreateIndex
CREATE INDEX "medias_property_id_idx" ON "property"."medias"("property_id");

-- CreateIndex
CREATE INDEX "rate_overrides_unit_id_idx" ON "property"."rate_overrides"("unit_id");

-- CreateIndex
CREATE INDEX "rate_overrides_date_range_id_idx" ON "property"."rate_overrides"("date_range_id");

-- CreateIndex
CREATE INDEX "rate_overrides_price_id_idx" ON "property"."rate_overrides"("price_id");

-- CreateIndex
CREATE INDEX "los_discounts_unit_id_idx" ON "property"."los_discounts"("unit_id");

-- CreateIndex
CREATE INDEX "los_discounts_amount_off_id_idx" ON "property"."los_discounts"("amount_off_id");

-- CreateIndex
CREATE INDEX "fees_unit_id_idx" ON "property"."fees"("unit_id");

-- CreateIndex
CREATE INDEX "fees_amount_id_idx" ON "property"."fees"("amount_id");

-- CreateIndex
CREATE INDEX "taxes_unit_id_idx" ON "property"."taxes"("unit_id");

-- CreateIndex
CREATE INDEX "unit_medias_unit_id_idx" ON "property"."unit_medias"("unit_id");

-- CreateIndex
CREATE INDEX "units_link_unit_id_idx" ON "property"."units_link"("unit_id");

-- CreateIndex
CREATE UNIQUE INDEX "units_link_property_id_unit_id_key" ON "property"."units_link"("property_id", "unit_id");

-- CreateIndex
CREATE INDEX "unit_applicable_promo_codes_promo_code_id_idx" ON "property"."unit_applicable_promo_codes"("promo_code_id");

-- CreateIndex
CREATE UNIQUE INDEX "unit_applicable_promo_codes_unit_id_promo_code_id_key" ON "property"."unit_applicable_promo_codes"("unit_id", "promo_code_id");

-- CreateIndex
CREATE UNIQUE INDEX "availability_exceptions_name_key" ON "schedule"."availability_exceptions"("name");

-- CreateIndex
CREATE INDEX "availability_exceptions_property_id_idx" ON "schedule"."availability_exceptions"("property_id");

-- CreateIndex
CREATE INDEX "availability_exceptions_unit_id_idx" ON "schedule"."availability_exceptions"("unit_id");

-- CreateIndex
CREATE INDEX "availability_exceptions_window_id_idx" ON "schedule"."availability_exceptions"("window_id");

-- CreateIndex
CREATE INDEX "availability_exceptions_date_range_id_idx" ON "schedule"."availability_exceptions"("date_range_id");

-- CreateIndex
CREATE UNIQUE INDEX "resource_name_key" ON "schedule"."resource"("name");

-- CreateIndex
CREATE INDEX "resource_property_id_idx" ON "schedule"."resource"("property_id");

-- CreateIndex
CREATE INDEX "resource_buffers_id_idx" ON "schedule"."resource"("buffers_id");

-- CreateIndex
CREATE INDEX "resource_stay_constraints_id_idx" ON "schedule"."resource"("stay_constraints_id");

-- CreateIndex
CREATE INDEX "resource_cancellation_policy_id_idx" ON "schedule"."resource"("cancellation_policy_id");

-- CreateIndex
CREATE INDEX "recurring_rules_schedule_id_idx" ON "schedule"."recurring_rules"("schedule_id");

-- CreateIndex
CREATE INDEX "refund_tiers_cancellation_policy_id_idx" ON "schedule"."refund_tiers"("cancellation_policy_id");

-- CreateIndex
CREATE INDEX "exceptions_availability_exception_id_idx" ON "schedule"."exceptions"("availability_exception_id");

-- CreateIndex
CREATE UNIQUE INDEX "exceptions_schedule_id_availability_exception_id_key" ON "schedule"."exceptions"("schedule_id", "availability_exception_id");

-- CreateIndex
CREATE UNIQUE INDEX "idempotency_keys_name_key" ON "shared"."idempotency_keys"("name");

-- CreateIndex
CREATE UNIQUE INDEX "idempotency_keys_method_request_id_key" ON "shared"."idempotency_keys"("method", "request_id");

-- CreateIndex
CREATE INDEX "price_components_booking_id_idx" ON "shared"."price_components"("booking_id");

-- CreateIndex
CREATE INDEX "price_components_amount_id_idx" ON "shared"."price_components"("amount_id");

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_unit_fkey" FOREIGN KEY ("unit") REFERENCES "property"."units"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_customer_fkey" FOREIGN KEY ("customer") REFERENCES "identity"."users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_promo_code_fkey" FOREIGN KEY ("promo_code") REFERENCES "promocode"."resource"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_contact_id_fkey" FOREIGN KEY ("contact_id") REFERENCES "shared"."contacts"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_occupancy_id_fkey" FOREIGN KEY ("occupancy_id") REFERENCES "booking"."occupancies"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_window_id_fkey" FOREIGN KEY ("window_id") REFERENCES "shared"."time_windows"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_price_id_fkey" FOREIGN KEY ("price_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_discount_id_fkey" FOREIGN KEY ("discount_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_total_id_fkey" FOREIGN KEY ("total_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "booking"."resource" ADD CONSTRAINT "resource_refund_amount_id_fkey" FOREIGN KEY ("refund_amount_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "channel"."resource" ADD CONSTRAINT "resource_property_fkey" FOREIGN KEY ("property") REFERENCES "property"."properties"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "channel"."unit_mappings" ADD CONSTRAINT "unit_mappings_unit_fkey" FOREIGN KEY ("unit") REFERENCES "property"."units"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "channel"."unit_mappings" ADD CONSTRAINT "unit_mappings_channel_id_fkey" FOREIGN KEY ("channel_id") REFERENCES "channel"."resource"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "identity"."guests" ADD CONSTRAINT "guests_booking_id_fkey" FOREIGN KEY ("booking_id") REFERENCES "booking"."resource"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "identity"."guests" ADD CONSTRAINT "guests_id_document_id_fkey" FOREIGN KEY ("id_document_id") REFERENCES "identity"."id_documents"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "identity"."guests" ADD CONSTRAINT "guests_permanent_address_id_fkey" FOREIGN KEY ("permanent_address_id") REFERENCES "common"."postal_address"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "identity"."guests" ADD CONSTRAINT "guests_local_address_id_fkey" FOREIGN KEY ("local_address_id") REFERENCES "common"."postal_address"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "identity"."guests" ADD CONSTRAINT "guests_foreigner_id_fkey" FOREIGN KEY ("foreigner_id") REFERENCES "identity"."foreigner_details"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "identity"."guests" ADD CONSTRAINT "guests_preferences_id_fkey" FOREIGN KEY ("preferences_id") REFERENCES "identity"."guest_preferences"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "identity"."id_documents" ADD CONSTRAINT "id_documents_attachment_id_fkey" FOREIGN KEY ("attachment_id") REFERENCES "shared"."attachments"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "organisation"."members" ADD CONSTRAINT "members_user_fkey" FOREIGN KEY ("user") REFERENCES "identity"."users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "organisation"."members" ADD CONSTRAINT "members_inviter_fkey" FOREIGN KEY ("inviter") REFERENCES "identity"."users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "organisation"."members" ADD CONSTRAINT "members_organisation_id_fkey" FOREIGN KEY ("organisation_id") REFERENCES "organisation"."resource"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."resource" ADD CONSTRAINT "resource_discount_id_fkey" FOREIGN KEY ("discount_id") REFERENCES "promocode"."discounts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."resource" ADD CONSTRAINT "resource_window_id_fkey" FOREIGN KEY ("window_id") REFERENCES "promocode"."redemption_windows"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."resource" ADD CONSTRAINT "resource_limits_id_fkey" FOREIGN KEY ("limits_id") REFERENCES "promocode"."usage_limits"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."resource" ADD CONSTRAINT "resource_scope_id_fkey" FOREIGN KEY ("scope_id") REFERENCES "promocode"."scopes"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."redemptions" ADD CONSTRAINT "redemptions_customer_fkey" FOREIGN KEY ("customer") REFERENCES "identity"."users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."redemptions" ADD CONSTRAINT "redemptions_booking_fkey" FOREIGN KEY ("booking") REFERENCES "booking"."resource"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."redemptions" ADD CONSTRAINT "redemptions_promo_code_id_fkey" FOREIGN KEY ("promo_code_id") REFERENCES "promocode"."resource"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."redemptions" ADD CONSTRAINT "redemptions_amount_applied_id_fkey" FOREIGN KEY ("amount_applied_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."discounts" ADD CONSTRAINT "discounts_amount_off_id_fkey" FOREIGN KEY ("amount_off_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."scopes" ADD CONSTRAINT "scopes_min_subtotal_id_fkey" FOREIGN KEY ("min_subtotal_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."scope_applicable_properties" ADD CONSTRAINT "scope_applicable_properties_scope_id_fkey" FOREIGN KEY ("scope_id") REFERENCES "promocode"."scopes"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."scope_applicable_properties" ADD CONSTRAINT "scope_applicable_properties_property_id_fkey" FOREIGN KEY ("property_id") REFERENCES "property"."properties"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."scope_applicable_units" ADD CONSTRAINT "scope_applicable_units_scope_id_fkey" FOREIGN KEY ("scope_id") REFERENCES "promocode"."scopes"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "promocode"."scope_applicable_units" ADD CONSTRAINT "scope_applicable_units_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."properties" ADD CONSTRAINT "properties_organisation_fkey" FOREIGN KEY ("organisation") REFERENCES "organisation"."resource"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."properties" ADD CONSTRAINT "properties_address_id_fkey" FOREIGN KEY ("address_id") REFERENCES "common"."postal_address"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."properties" ADD CONSTRAINT "properties_policy_id_fkey" FOREIGN KEY ("policy_id") REFERENCES "property"."policies"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."units" ADD CONSTRAINT "units_property_id_fkey" FOREIGN KEY ("property_id") REFERENCES "property"."properties"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."units" ADD CONSTRAINT "units_price_id_fkey" FOREIGN KEY ("price_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."licences" ADD CONSTRAINT "licences_unit_fkey" FOREIGN KEY ("unit") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."licences" ADD CONSTRAINT "licences_property_id_fkey" FOREIGN KEY ("property_id") REFERENCES "property"."properties"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."licences" ADD CONSTRAINT "licences_attachment_id_fkey" FOREIGN KEY ("attachment_id") REFERENCES "shared"."attachments"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."medias" ADD CONSTRAINT "medias_property_id_fkey" FOREIGN KEY ("property_id") REFERENCES "property"."properties"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."rate_overrides" ADD CONSTRAINT "rate_overrides_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."rate_overrides" ADD CONSTRAINT "rate_overrides_date_range_id_fkey" FOREIGN KEY ("date_range_id") REFERENCES "shared"."date_ranges"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."rate_overrides" ADD CONSTRAINT "rate_overrides_price_id_fkey" FOREIGN KEY ("price_id") REFERENCES "common"."moneys"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."los_discounts" ADD CONSTRAINT "los_discounts_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."los_discounts" ADD CONSTRAINT "los_discounts_amount_off_id_fkey" FOREIGN KEY ("amount_off_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."fees" ADD CONSTRAINT "fees_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."fees" ADD CONSTRAINT "fees_amount_id_fkey" FOREIGN KEY ("amount_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."taxes" ADD CONSTRAINT "taxes_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."unit_medias" ADD CONSTRAINT "unit_medias_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."units_link" ADD CONSTRAINT "units_link_property_id_fkey" FOREIGN KEY ("property_id") REFERENCES "property"."properties"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."units_link" ADD CONSTRAINT "units_link_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."unit_applicable_promo_codes" ADD CONSTRAINT "unit_applicable_promo_codes_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "property"."unit_applicable_promo_codes" ADD CONSTRAINT "unit_applicable_promo_codes_promo_code_id_fkey" FOREIGN KEY ("promo_code_id") REFERENCES "promocode"."resource"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."availability_exceptions" ADD CONSTRAINT "availability_exceptions_property_id_fkey" FOREIGN KEY ("property_id") REFERENCES "property"."properties"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."availability_exceptions" ADD CONSTRAINT "availability_exceptions_unit_id_fkey" FOREIGN KEY ("unit_id") REFERENCES "property"."units"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."availability_exceptions" ADD CONSTRAINT "availability_exceptions_window_id_fkey" FOREIGN KEY ("window_id") REFERENCES "shared"."time_windows"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."availability_exceptions" ADD CONSTRAINT "availability_exceptions_date_range_id_fkey" FOREIGN KEY ("date_range_id") REFERENCES "shared"."date_ranges"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."resource" ADD CONSTRAINT "resource_property_id_fkey" FOREIGN KEY ("property_id") REFERENCES "property"."properties"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."resource" ADD CONSTRAINT "resource_buffers_id_fkey" FOREIGN KEY ("buffers_id") REFERENCES "schedule"."buffer_settings"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."resource" ADD CONSTRAINT "resource_stay_constraints_id_fkey" FOREIGN KEY ("stay_constraints_id") REFERENCES "schedule"."stay_constraints"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."resource" ADD CONSTRAINT "resource_cancellation_policy_id_fkey" FOREIGN KEY ("cancellation_policy_id") REFERENCES "schedule"."cancellation_policies"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."recurring_rules" ADD CONSTRAINT "recurring_rules_schedule_id_fkey" FOREIGN KEY ("schedule_id") REFERENCES "schedule"."resource"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."refund_tiers" ADD CONSTRAINT "refund_tiers_cancellation_policy_id_fkey" FOREIGN KEY ("cancellation_policy_id") REFERENCES "schedule"."cancellation_policies"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."exceptions" ADD CONSTRAINT "exceptions_schedule_id_fkey" FOREIGN KEY ("schedule_id") REFERENCES "schedule"."resource"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "schedule"."exceptions" ADD CONSTRAINT "exceptions_availability_exception_id_fkey" FOREIGN KEY ("availability_exception_id") REFERENCES "schedule"."availability_exceptions"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "shared"."price_components" ADD CONSTRAINT "price_components_booking_id_fkey" FOREIGN KEY ("booking_id") REFERENCES "booking"."resource"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "shared"."price_components" ADD CONSTRAINT "price_components_amount_id_fkey" FOREIGN KEY ("amount_id") REFERENCES "common"."moneys"("id") ON DELETE SET NULL ON UPDATE CASCADE;
