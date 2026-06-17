use super::validate::*;
use serde_json::json;

fn has_error(result: &ValidationResult, field: &str, code: &str) -> bool {
    result
        .errors
        .iter()
        .any(|error| error.field == field && error.code == code)
}

fn assert_error(result: &ValidationResult, field: &str, code: &str) {
    assert!(
        has_error(result, field, code),
        "expected {field}:{code} in {:?}",
        result.errors
    );
}

#[test]
fn validation_result_valid_starts_clean() {
    let result = ValidationResult::valid();

    assert!(result.valid);
    assert!(!result.has_errors());
    assert!(!result.has_warnings());
    assert!(result.errors.is_empty());
    assert!(result.warnings.is_empty());
}

#[test]
fn validation_result_error_records_error_severity() {
    let result = ValidationResult::error("side", "required", "Side is required");

    assert!(!result.valid);
    assert!(result.has_errors());
    assert_eq!(result.errors[0].field, "side");
    assert_eq!(result.errors[0].code, "required");
    assert_eq!(result.errors[0].severity, Severity::Error);
}

#[test]
fn validation_result_combine_merges_errors_and_warnings() {
    let mut result = ValidationResult::valid();
    result.add_warning("deprecated field");

    let mut other = ValidationResult::error("price", "invalid_price", "Price must be positive");
    other.add_warning("legacy message type");
    result.combine(other);

    assert!(!result.valid);
    assert!(result.has_errors());
    assert!(result.has_warnings());
    assert_error(&result, "price", "invalid_price");
    assert_eq!(
        result.warnings,
        vec!["deprecated field", "legacy message type"]
    );
}

#[test]
fn required_validator_accepts_some_and_rejects_none() {
    let validator = RequiredValidator;

    assert!(validator.validate(&Some("buy"), "side").valid);

    let missing: Option<&str> = None;
    let result = validator.validate(&missing, "side");
    assert!(!result.valid);
    assert_error(&result, "side", "required");
}

#[test]
fn string_length_validator_checks_minimum_and_maximum() {
    let validator = StringLengthValidator {
        min: Some(3),
        max: Some(5),
    };

    assert!(validator.validate(&"ETH".to_string(), "symbol").valid);
    assert_error(
        &validator.validate(&"X".to_string(), "symbol"),
        "symbol",
        "min_length",
    );
    assert_error(
        &validator.validate(&"BTCUSDT".to_string(), "symbol"),
        "symbol",
        "max_length",
    );
}

#[test]
fn numeric_range_validator_checks_minimum_and_maximum() {
    let validator = NumericRangeValidator {
        min: Some(1.0),
        max: Some(10.0),
    };

    assert!(validator.validate(&5.0, "quantity").valid);
    assert_error(
        &validator.validate(&0.5, "quantity"),
        "quantity",
        "min_value",
    );
    assert_error(
        &validator.validate(&10.5, "quantity"),
        "quantity",
        "max_value",
    );
}

#[test]
fn regex_validator_accepts_match_and_rejects_mismatch() {
    let validator = RegexValidator {
        pattern: r"^[A-Z]{3}/[A-Z]{3}$",
    };

    assert!(validator.validate(&"BTC/USD".to_string(), "symbol").valid);
    assert_error(
        &validator.validate(&"btc-usd".to_string(), "symbol"),
        "symbol",
        "pattern_mismatch",
    );
}

#[test]
fn enum_validator_accepts_known_variant_and_rejects_unknown() {
    let validator = EnumValidator {
        variants: &["market", "limit"],
    };

    assert!(validator.validate(&"limit".to_string(), "type").valid);
    assert_error(
        &validator.validate(&"iceberg".to_string(), "type"),
        "type",
        "invalid_value",
    );
}

#[test]
fn email_validator_accepts_valid_and_rejects_invalid() {
    let validator = EmailValidator;

    assert!(
        validator
            .validate(&"trader@example.com".to_string(), "email")
            .valid
    );
    assert_error(
        &validator.validate(&"missing-at.example.com".to_string(), "email"),
        "email",
        "invalid_email",
    );
}

#[test]
fn message_validator_reports_schema_stage_mismatch_without_registered_schema() {
    let validator = MessageValidator::new();

    let result = validator.validate(42, 1, br#"{"side":"buy"}"#);

    assert!(!result.valid);
    assert_error(&result, "_schema", "schema_mismatch");
}

#[test]
fn message_validator_runs_registered_field_validators_on_json_payload() {
    let mut validator = MessageValidator::new();
    validator.register_field_validator(
        7,
        Box::new(|payload| {
            let mut result = ValidationResult::valid();
            if payload
                .get("client_id")
                .and_then(|value| value.as_str())
                .is_none()
            {
                result.add_error("client_id", "required", "client_id is required");
            }
            result
        }),
    );

    let result = validator.validate(7, 1, br#"{"side":"buy"}"#);

    assert_error(&result, "_schema", "schema_mismatch");
    assert_error(&result, "client_id", "required");
}

#[test]
fn message_validator_skips_field_validators_for_non_json_payload() {
    let mut validator = MessageValidator::new();
    validator.register_field_validator(
        7,
        Box::new(|_| ValidationResult::error("client_id", "required", "client_id is required")),
    );

    let result = validator.validate(7, 1, b"not-json");

    assert_error(&result, "_schema", "schema_mismatch");
    assert!(!has_error(&result, "client_id", "required"));
}

#[test]
fn message_validator_runs_custom_integrity_validator_for_checksum_failure() {
    let mut validator = MessageValidator::new();
    validator.register_custom_validator(Box::new(|message_type, payload| {
        let value: serde_json::Value =
            serde_json::from_slice(payload).unwrap_or_else(|_| json!({}));
        let mut result = ValidationResult::valid();
        if message_type != 9 {
            result.add_error(
                "_integrity",
                "unexpected_message_type",
                "message type mismatch",
            );
        }
        if value.get("checksum").and_then(|checksum| checksum.as_str()) != Some("ok") {
            result.add_error(
                "checksum",
                "checksum_mismatch",
                "checksum does not match payload",
            );
        }
        result
    }));

    let failed = validator.validate(9, 1, br#"{"checksum":"bad"}"#);
    let passed = validator.validate(9, 1, br#"{"checksum":"ok"}"#);

    assert_error(&failed, "checksum", "checksum_mismatch");
    assert!(!has_error(&passed, "checksum", "checksum_mismatch"));
}

#[test]
fn validate_order_payload_accepts_valid_market_order_without_price() {
    let payload = json!({
        "side": "buy",
        "type": "market",
        "quantity": 10.5,
        "time_in_force": "ioc"
    });

    let result = MessageValidator::validate_order_payload(&payload);

    assert!(result.valid, "{:?}", result.errors);
}

#[test]
fn validate_order_payload_reports_missing_required_fields() {
    let result = MessageValidator::validate_order_payload(&json!({}));

    assert_error(&result, "side", "required");
    assert_error(&result, "type", "required");
    assert_error(&result, "quantity", "required");
    assert_error(&result, "price", "required");
}

#[test]
fn validate_order_payload_rejects_invalid_enums() {
    let payload = json!({
        "side": "hold",
        "type": "iceberg",
        "quantity": 10.0,
        "price": 20.0,
        "time_in_force": "forever"
    });

    let result = MessageValidator::validate_order_payload(&payload);

    assert_error(&result, "side", "invalid_side");
    assert_error(&result, "type", "invalid_type");
    assert_error(&result, "time_in_force", "invalid_tif");
}

#[test]
fn validate_order_payload_rejects_quantity_range_violations() {
    let zero_quantity = json!({
        "side": "sell",
        "type": "market",
        "quantity": 0.0
    });
    let oversized_quantity = json!({
        "side": "sell",
        "type": "market",
        "quantity": 1_000_000.01
    });

    assert_error(
        &MessageValidator::validate_order_payload(&zero_quantity),
        "quantity",
        "invalid_quantity",
    );
    assert_error(
        &MessageValidator::validate_order_payload(&oversized_quantity),
        "quantity",
        "max_exceeded",
    );
}

#[test]
fn validate_order_payload_requires_positive_price_for_non_market_orders() {
    let missing_price = json!({
        "side": "buy",
        "type": "limit",
        "quantity": 1.0
    });
    let negative_price = json!({
        "side": "buy",
        "type": "stop",
        "quantity": 1.0,
        "price": -1.0
    });

    assert_error(
        &MessageValidator::validate_order_payload(&missing_price),
        "price",
        "required",
    );
    assert_error(
        &MessageValidator::validate_order_payload(&negative_price),
        "price",
        "invalid_price",
    );
}

#[test]
fn validate_order_payload_rejects_type_mismatches_for_numeric_fields() {
    let payload = json!({
        "side": "buy",
        "type": "limit",
        "quantity": "10",
        "price": "200.50"
    });

    let result = MessageValidator::validate_order_payload(&payload);

    assert_error(&result, "quantity", "required");
    assert_error(&result, "price", "required");
}

#[test]
fn validate_account_payload_accepts_valid_amount_and_currency() {
    let payload = json!({
        "amount": 2500.25,
        "currency": "USDC"
    });

    let result = MessageValidator::validate_account_payload(&payload);

    assert!(result.valid, "{:?}", result.errors);
}

#[test]
fn validate_account_payload_rejects_amount_and_currency_violations() {
    let invalid_amount = json!({
        "amount": -1.0,
        "currency": "USD"
    });
    let oversized_amount = json!({
        "amount": 1_000_000_000.01,
        "currency": "USD"
    });
    let unsupported_currency = json!({
        "amount": 10.0,
        "currency": "DOGE"
    });

    assert_error(
        &MessageValidator::validate_account_payload(&invalid_amount),
        "amount",
        "invalid_amount",
    );
    assert_error(
        &MessageValidator::validate_account_payload(&oversized_amount),
        "amount",
        "max_exceeded",
    );
    assert_error(
        &MessageValidator::validate_account_payload(&unsupported_currency),
        "currency",
        "invalid_currency",
    );
}

#[test]
fn convenience_email_phone_uuid_and_hex_validators_cover_edges() {
    assert!(validate_email("desk@example.com"));
    assert!(!validate_email("desk.example.com"));
    assert!(validate_phone("+1 (415) 555-0100"));
    assert!(!validate_phone("555"));
    assert!(validate_uuid("123e4567-e89b-12d3-a456-426614174000"));
    assert!(!validate_uuid("123E4567-E89B-12D3-A456-426614174000"));
    assert!(validate_hex_string("deadBEEF", 4));
    assert!(!validate_hex_string("deadbee", 4));
}

#[test]
fn convenience_timestamp_symbol_and_instrument_validators_cover_edges() {
    assert!(validate_timestamp(946684800000));
    assert!(validate_timestamp(4102444800000));
    assert!(!validate_timestamp(946684799999));
    assert!(!validate_timestamp(4102444800001));
    assert!(validate_symbol("BTC/USD"));
    assert!(!validate_symbol("btc/usd"));
    assert!(validate_instrument_id("btcperp01"));
    assert!(!validate_instrument_id("BTC-PERP-01"));
}

#[test]
fn convenience_price_and_quantity_validators_cover_boundaries() {
    assert!(validate_price(1.0));
    assert!(validate_price(0.000000001));
    assert!(!validate_price(0.0));
    assert!(!validate_price(1_000_000_000.0));
    assert!(!validate_price(1.0000000005));
    assert!(validate_quantity(99_999_999.99));
    assert!(!validate_quantity(0.0));
    assert!(!validate_quantity(100_000_000.0));
}

#[test]
fn compliance_auditor_duplicate_amount_rule_diverges_from_rust_validation() {
    let compliance_source = include_str!("../../../compliance/ComplianceAuditor.java");
    let rust_source = include_str!("validate.rs");
    let threshold_line = compliance_source
        .lines()
        .find(|line| line.contains("double threshold ="))
        .expect("ComplianceAuditor should define the AML threshold");
    let threshold = threshold_line
        .split('=')
        .nth(1)
        .expect("threshold assignment should have a value")
        .trim()
        .trim_end_matches(';')
        .parse::<f64>()
        .expect("AML threshold should be numeric");
    let amount = threshold + 0.01;

    let rust_result = MessageValidator::validate_account_payload(&json!({
        "amount": amount,
        "currency": "USD"
    }));

    assert!(rust_source.contains("business validation rules are duplicated"));
    assert!(compliance_source.contains("auditAML"));
    assert!(compliance_source.contains("transaction_amount"));
    assert!(
        rust_result.valid,
        "Rust account validation accepts amounts that the Java AML rule would flag: {:?}",
        rust_result.errors
    );
    assert!(
        amount > threshold,
        "the test amount should cross ComplianceAuditor's AML threshold"
    );
}
