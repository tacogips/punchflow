# Design References

This directory contains reference materials for system design and implementation.

## External References

| Name | URL | Description |
|------|-----|-------------|
| Go Documentation | https://go.dev/doc/ | Official Go documentation |
| Effective Go | https://go.dev/doc/effective_go | Go best practices and idioms |
| Standard Go Project Layout | https://github.com/golang-standards/project-layout | Standard project structure conventions |
| kickflow Developer | https://developer.kickflow.com/ | Official kickflow REST API, Webhook and MCP documentation |
| kickflow Help Center | https://support.kickflow.com/hc/ja | Official kickflow product manual (Japanese) |

## kickflow (dummy server target)

| File | Description |
|------|-------------|
| [kickflow-api.md](./kickflow-api.md) | Consolidated English reference: API conventions, all 96 REST operations, all schemas, webhooks, UI feature inventory, confirmed vs inferred |
| [kickflow-openapi.yaml](./kickflow-openapi.yaml) | Official OpenAPI 3.1 definition (MIT), copied from https://developer.kickflow.com/rest/schema.yaml on 2026-10-20 |

## Reference Documents

Reference documents should be organized by topic:

```
references/
├── README.md              # This index file
├── golang/                # Go patterns and practices
└── <topic>/               # Other topic-specific references
```

## Adding References

When adding new reference materials:

1. Create a topic directory if it does not exist
2. Add reference documents with clear naming
3. Update this README.md with the reference entry
