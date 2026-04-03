# Sub-Question 3 Findings: Product Metadata & Configuration

**Source 1:** 12 Factor App - Configuration
**URL:** https://12factor.net/config
**Accessed:** 2026-03-19

## Key Findings

### 12-Factor App Principles
- Store configuration in the environment (environment variables)
- Env vars are granular controls, each fully orthogonal to other env vars
- Never group configurations as "environments" (dev, staging, prod)
- Each deploy has its own environment variable set
- Configuration should be separated from code

### Project Folder Organization (Iterators)
**URL:** https://www.iteratorshq.com/blog/a-comprehensive-guide-on-project-folder-organization/
**Accessed:** 2026-03-19

- Common practice: `.env` file for environment variables (excluded from version control)
- Configuration files at root level: `package.json`, `requirements.txt`, `go.mod`
- Separation of concerns:
  - `config/` folder for configuration files
  - `scripts/` for deployment and build scripts
  - `deployments/` or `infrastructure/` for IaC (Terraform, CloudFormation)
- Different environments: `config/development.yaml`, `config/production.yaml`

### Configuration File Management (Software Engineering Stack Exchange)
**URL:** https://softwareengineering.stackexchange.com/questions/283715/what-is-the-preferred-way-to-store-application-configurations
**Accessed:** 2026-03-19

- Debate: config files vs environment variables
- 12 Factor App recommends environment variables for security and portability
- Development config often in root directory but risks leaking secrets
- Best practice: Use environment variables for sensitive data, config files for non-sensitive defaults

### Example Patterns from Search Results

1. **Python projects**: `config/` folder with `default.py`, `production.py`, `development.py`
2. **Node.js projects**: `.env` file + `config/` folder with JSON/YAML files
3. **Go projects**: `configs/` or `internal/config/` package
4. **Docker/containerized**: ConfigMaps and Secrets mounted as volumes or env vars

## Patterns Identified

1. **Separation of config from code**: Configuration should not be embedded in code
2. **Environment-specific configuration**: Different values per environment (dev, staging, prod)
3. **Secret management**: Sensitive data via environment variables or secret managers
4. **Hierarchical configuration**: Defaults → Environment overrides → Local overrides
5. **Infrastructure as Code**: Deployment configurations separate from application config

## Implications for Digital Product Structure
- Need `config/` or `configuration/` folder for non-sensitive configuration files
- `.env.example` or `.env.template` for required environment variables
- `deploy/` or `infrastructure/` for deployment scripts and IaC
- `scripts/` for build, test, and utility scripts
- Product metadata (name, version, description) in `package.json`, `pyproject.toml`, or dedicated `product.yaml`