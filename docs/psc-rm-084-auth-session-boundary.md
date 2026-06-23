# PSC-RM-084 Authenticated Session Boundary

Internal lab-development note. This is not a production-readiness claim and does not authorize deployment, external exposure, DNS/auth changes, customer data use, or security-account mutation.

## Boundary

HTTP requests now use an internal server-side session token boundary:

- The browser/client presents only `psc_internal_session`, an opaque cookie token.
- Tenant/lab scope and actor identity come from the server-side session record mapped to that token.
- Caller-selected `X-PSC-Tenant-ID`, `X-PSC-Lab-ID`, `tenant_id`, `lab_id`, or actor form/header values are not trusted as identity or scope claims.
- If a request attempts to select a tenant/lab outside the authenticated session, the HTTP layer fails closed before mutation.
- Missing, unknown, expired, or invalid session records are denied with `401 Unauthorized`.

## Lab-validation configuration

For local/internal lab validation, configure an explicit internal session token in the process environment before starting `serve`:

```bash
export PSC_INTERNAL_SESSION_TOKEN='<opaque-random-token>'
export PSC_INTERNAL_SESSION_USER='lab-dev'
export PSC_INTERNAL_SESSION_TENANT_ID='lab-test'
export PSC_INTERNAL_SESSION_LAB_ID='default-lab'
export PSC_INTERNAL_SESSION_TTL='12h'
```

Then requests must include:

```http
Cookie: psc_internal_session=<opaque-random-token>
```

The token is only a lookup key. It does not carry tenant/lab/role claims itself.

## Validation added

Regression coverage includes:

- missing session denial;
- expired session denial;
- caller-selected cross-tenant scope denial from header/form/query inputs;
- positive path proving new records inherit tenant/lab from the trusted server-side session;
- existing spoofed actor header/form tests continue to verify audit identity is not caller-controlled.

## Non-goals

This remediation does not add external SSO, production session persistence, CSRF protection, customer-facing authentication, deployment, or public exposure. Those remain separate security/design gates.
