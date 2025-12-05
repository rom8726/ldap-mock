# ldap-mock

`ldap-mock` is a testing tool for Go projects that allows you to mock LDAP server interactions (e.g., Active Directory).
It enables you to create and manage LDAP request mocks efficiently through a simple HTTP API, making development and testing more straightforward.
Code uses [github.com/bradleypeabody/godap](github.com/bradleypeabody/godap).

## Features
- Run an LDAP mock server on a specified port (default: `389`).
- Manage mocks (creation and cleanup) via HTTP API.
- Supports YAML format for loading mocks.
- **Rule-based matching** — define rules to return different responses based on LDAP filter, BaseDN, and scope.
- **Priority-based rule evaluation** — rules with higher priority are evaluated first.
- Easily integratable into your tests.

## Getting Started

### Run with Docker
You can use Docker to deploy `ldap-mock`:

```sh
docker run -p 389:389 -p 6006:6006 -e LDAP_USERNAME=admin -e LDAP_PASSWORD=admin123 rom8726/ldap-mock:latest
```

Environment variables:
- `LDAP_PORT` — Port for the LDAP server (default: `389`).
- `MOCK_PORT` — Port for the mock HTTP server (default: `6006`).
- `LDAP_USERNAME` — Username for binding to the LDAP server.
- `LDAP_PASSWORD` — Password for binding to the LDAP server.

### HTTP API
`ldap-mock` provides an HTTP API (on port `6006`) for managing mocks:

#### Load Mocks
To load user mocks, send the following request:

```shell
curl -X POST http://localhost:6006/mock \
     -H "Content-Type: application/x-yaml" \
     -d '
users:
  - cn: CN=John.Doe,OU=Users,DC=example,DC=com
    attrs:
      name: John Doe
      telephoneNumber: +1234567890
      mail: john.doe@example.com
      title: Software Engineer
'
```

#### Clear Mocks
To clear all currently loaded mocks:

```shell
curl -X POST http://localhost:6006/clean
```


## Mocks Format

### Basic Format (Fallback Users)

The simplest way to define mocks — all LDAP searches will return these users:

```yaml
users:
  - cn: CN=John.Doe,OU=Users,DC=example,DC=com
    attrs:
      mail: john.doe@example.com
      title: Software Engineer
  - cn: CN=Alice.Smith,OU=HR,DC=example,DC=com
    attrs:
      mail: alice.smith@example.com
      department: Human Resources
```

### Rule-Based Format

For more control, define rules that match specific LDAP queries:

```yaml
users:
  - cn: fallback-user
    attrs:
      mail: fallback@example.com

rules:
  - name: John search
    filter: "(cn=john)"
    response:
      users:
        - cn: CN=John.Doe,OU=Users,DC=example,DC=com
          attrs:
            mail: john@example.com
            title: Developer

  - name: Engineering team
    filter: "(department=engineering)"
    base_dn: "DC=example,DC=com"
    scope: "sub"
    priority: 10
    response:
      users:
        - cn: CN=Dev1,OU=Engineering,DC=example,DC=com
          attrs:
            mail: dev1@example.com
        - cn: CN=Dev2,OU=Engineering,DC=example,DC=com
          attrs:
            mail: dev2@example.com
```

### Rule Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | No | Human-readable rule name (for logging) |
| `filter` | Yes | LDAP filter to match (RFC 4515 syntax) |
| `base_dn` | No | Match only if request BaseDN equals this value |
| `scope` | No | Match only if request scope equals: `base`, `one`, or `sub` |
| `priority` | No | Higher priority rules are evaluated first (default: 0) |
| `response` | Yes | Response to return when rule matches |

### How Matching Works

1. When an LDAP search request arrives, rules are evaluated in **priority order** (highest first).
2. For each rule:
   - If `base_dn` is specified, it must match the request's BaseDN.
   - If `scope` is specified, it must match the request's scope.
   - The `filter` must match the request's filter.
3. **First matching rule wins** — its `response.users` are returned.
4. If no rule matches, **fallback `users`** are returned (filtered by the request filter).

### Supported Filter Syntax

- Equality: `(cn=John)`
- Presence: `(mail=*)`
- Approximate: `(cn~=John)`
- Comparison: `(age>=18)`, `(age<=65)`
- AND: `(&(cn=John)(mail=*))`
- OR: `(|(cn=John)(cn=Jane))`
- NOT: `(!(cn=John))`


## Usage in Tests

1. Start `ldap-mock` (using Docker, for example).
2. Load mocks via the HTTP API before running your test.
3. Interact with `ldap-mock` as a regular LDAP server (default port: `389`).
4. Clear mocks via the HTTP API to reuse the setup in subsequent tests.

### docker-compose.yml example

```yaml
version: "3.9"
services:
  ldap-mock:
    image: rom8726/ldap-mock:latest
    ports:
      - "389:389"
      - "6006:6006"
    environment:
      LDAP_USERNAME: admin
      LDAP_PASSWORD: admin123
```

## UI (WireMock-style dashboard)

The dashboard is embedded and served at `http://<MOCK_HOST>:6006/ui`.

- **Requests**: recent LDAP requests, matched rule (if any), response counts; click a row to inspect details (request, rule match, response DNs).
- **Rules**: loaded rules and the current YAML.
- **Mock Data**: current users/attributes.

### Quick local run with docker-compose (dev helper)

`dev/docker-compose.yml` includes `ldap-mock` plus a `tester` that loads `dev/mock.yaml` and performs a couple of LDAP searches (including a rule-matching query) so the UI is populated immediately.

```
make compose-up   # build & start
make compose-down # stop & remove
```

### Example: Different responses for different queries

```yaml
rules:
  - name: Admin users
    filter: "(memberOf=CN=Admins,DC=example,DC=com)"
    priority: 10
    response:
      users:
        - cn: CN=Admin,OU=Users,DC=example,DC=com
          attrs:
            mail: admin@example.com
            role: admin

  - name: Regular users
    filter: "(objectClass=user)"
    priority: 1
    response:
      users:
        - cn: CN=User1,OU=Users,DC=example,DC=com
          attrs:
            mail: user1@example.com
        - cn: CN=User2,OU=Users,DC=example,DC=com
          attrs:
            mail: user2@example.com
```

## License
This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

Happy testing with `ldap-mock`!
