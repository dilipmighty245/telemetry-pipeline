# Documentation Index

This directory contains comprehensive documentation for the Elastic GPU Telemetry Pipeline.

## 📚 Available Documentation

### Core Documentation

- **[Architecture Guide](ARCHITECTURE.md)** - Complete system architecture, components, data flow, and design patterns
- **[API Specification](API_SPECIFICATION.md)** - Detailed REST API documentation with examples and data models
- **[Scaling & Kubernetes](SCALING_AND_KUBERNETES.md)** - Local scaling and Kubernetes deployment guide
- **[Debugging Guide](DEBUGGING.md)** - Comprehensive troubleshooting and debugging instructions

### Integration Documentation

- **[Nexus Integration Guide](NEXUS_INTEGRATION_GUIDE.md)** - Nexus framework integration details and patterns

### API Specifications

- **[swagger.json](swagger.json)** - OpenAPI 3.0 specification in JSON format
- **[swagger.yaml](swagger.yaml)** - OpenAPI 3.0 specification in YAML format

## 🚀 Quick Navigation

### For Developers
1. Start with **[Architecture Guide](ARCHITECTURE.md)** to understand the system
2. Follow **[Main README](../README.md)** for local development setup
3. Use **[Debugging Guide](DEBUGGING.md)** when troubleshooting issues

### For Operators  
1. Review **[Scaling & Kubernetes](SCALING_AND_KUBERNETES.md)** for deployment
2. Reference **[API Specification](API_SPECIFICATION.md)** for API usage
3. Use **[Debugging Guide](DEBUGGING.md)** for operational issues

### For API Users
1. Check **[API Specification](API_SPECIFICATION.md)** for complete API reference
2. Test APIs using examples in **[Main README](../README.md)**
3. Use **[swagger.json](swagger.json)** for API client generation

## 📖 Documentation Overview

### What Each Document Covers

| Document | Purpose | Audience |
|----------|---------|----------|
| [Architecture](ARCHITECTURE.md) | System design, components, data flow | Developers, Architects |
| [API Specification](API_SPECIFICATION.md) | REST API reference, examples | API Users, Frontend Devs |
| [Scaling & Kubernetes](SCALING_AND_KUBERNETES.md) | Deployment, scaling, operations | DevOps, SRE |
| [Debugging](DEBUGGING.md) | Troubleshooting, monitoring | Developers, Operators |
| [Nexus Integration](NEXUS_INTEGRATION_GUIDE.md) | Framework integration details | Nexus Developers |

### Key Features Documented

- ✅ **Local Development**: Complete setup and testing instructions
- ✅ **Kubernetes Deployment**: Production-ready Helm charts and configurations
- ✅ **Scaling**: Both local and Kubernetes horizontal scaling
- ✅ **API Reference**: All 20+ REST endpoints, GraphQL, and WebSocket APIs
- ✅ **Debugging**: Comprehensive troubleshooting for all components
- ✅ **Architecture**: System design patterns and data flow
- ✅ **Monitoring**: Health checks, logging, and performance monitoring

## 🔗 External References

- **[Main README](../README.md)** - Project overview and quick start
- **[Makefile](../Makefile)** - All available commands and targets
- **[Helm Charts](../deployments/helm/)** - Kubernetes deployment configurations
- **[Docker Files](../deployments/docker/)** - Container build configurations

## 📝 Contributing to Documentation

When updating documentation:

1. **Keep it current** - Update docs when making code changes
2. **Be comprehensive** - Include examples and use cases
3. **Cross-reference** - Link between related documents
4. **Test examples** - Verify all code examples work
5. **Update this index** - Add new documents to the table above

### Documentation Standards

- Use clear headings and structure
- Include practical examples
- Provide troubleshooting steps
- Link to related sections
- Keep examples up-to-date with current API

---

**Need help?** Start with the [Main README](../README.md) or check the [Debugging Guide](DEBUGGING.md) for troubleshooting.
