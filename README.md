# ldap-mock

`ldap-mock` is a testing tool for Go projects that allows you to mock LDAP server interactions (e.g., Active Directory).
It enables you to create and manage LDAP request mocks efficiently through a simple HTTP API, making development and testing more straightforward.
Code uses [github.com/bradleypeabody/godap](github.com/bradleypeabody/godap).

## Features
- Run an LDAP mock server on a specified port (default: `389`).
- Manage mocks (creation and cleanup) via HTTP API.
- Supports YAML format for loading mocks.
- Easily integratable into your tests.

## Getting Started

### Run with Docker
You can use Docker to deploy `ldap-mock`:

```sh
docker build -t ldap-mock .
docker run -p 389:389 -p 6006:6006 -e LDAP_USERNAME=admin -e LDAP_PASSWORD=admin123 ldap-mock
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
  - cn: CN=Alice.Smith,OU=HR,DC=example,DC=com
    attrs:
      name: Alice Smith
      telephoneNumber: +9876543210
      mail: alice.smith@example.com
      department: Human Resources
'
```

#### Clear Mocks
To clear all currently loaded mocks:

```shell
curl -X POST http://localhost:6006/clean
```


### Mocks Format
Mocks are provided in YAML format using the following structure:

```yaml
users:
  - cn: <fully-qualified-distinguished-name>
    attrs:
      <attribute>: <value>
```

#### Example
```yaml
users:
  - cn: CN=Michael.Jones,OU=Engineering,DC=example,DC=com
    attrs:
      mail: michael.jones@example.com
      telephoneNumber: "+1112223333"
      department: Engineering
  - cn: CN=Sarah.Williams,OU=Sales,DC=example,DC=com
    attrs:
      mail: sarah.williams@example.com
      mobile: "+4445556666"
      title: Sales Manager
```

### Usage in Tests
1. Start `ldap-mock` (using Docker, for example).
2. Load mocks via the HTTP API before running your test.
3. Interact with `ldap-mock` as a regular LDAP server (default port: `389`).
4. Clear mocks via the HTTP API to reuse the setup in subsequent tests.

#### docker-compose.yml example
You can use this docker-compose.yml example:

```yaml
version: "3.9"
services:
  ldap-mock:
    image: rom8726/ldap-mock:v0.1.0
    ports:
      - "389:389"     # Expose the LDAP server port
      - "6006:6006"   # Expose the HTTP API port for managing mocks
    environment:
      LDAP_USERNAME: admin            # Set the username for LDAP binding
      LDAP_PASSWORD: admin123         # Set the password for LDAP binding
      LDAP_PORT: 389                  # Port for the LDAP server (optional, default is 389)
      MOCK_PORT: 6006                 # Port for the mock HTTP server (optional, default is 6006)
```

## License
This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

Happy testing with `ldap-mock`!
