#!/bin/bash

# SpecGuard CLI Usage Examples

echo "🚀 SpecGuard Standalone CLI Examples"
echo "===================================="
echo ""

echo "📁 Scan any repository for API specs:"
echo "  specguard scan /path/to/your/api-repo"
echo ""

echo "🔍 Detect drift between specs and live APIs:"
echo "  specguard drift /path/to/your/api-repo"
echo "  specguard drift /path/to/your/api-repo --compare-with=production"
echo "  specguard drift /path/to/your/api-repo --compare-with=staging"
echo ""

echo "📚 Generate documentation (REST + gRPC):"
echo "  specguard generate /path/to/your/api-repo --output=./docs --formats=docs"
echo ""

echo "🔧 Generate SDKs for multiple languages:"
echo "  specguard generate /path/to/your/api-repo --output=./sdks --formats=sdk"
echo ""

echo "📦 Generate protobuf files:"
echo "  specguard generate /path/to/your/api-repo --output=./proto --formats=proto"
echo ""

echo "🎯 Generate everything at once:"
echo "  specguard generate /path/to/your/api-repo --output=./output --formats=docs,sdk,proto"
echo ""

echo "🌐 Start web server for API access:"
echo "  specguard server"
echo ""

echo "🗄️  Run database migrations:"
echo "  specguard migrate"
echo ""

echo "📖 What SpecGuard detects:"
echo "  ✅ OpenAPI (JSON/YAML) specifications"
echo "  ✅ Protocol Buffer (.proto) files"
echo "  ✅ gRPC service definitions"
echo "  ✅ REST endpoint drift"
echo "  ✅ gRPC service inconsistencies"
echo "  ✅ Protobuf naming conflicts"
echo "  ✅ Live API vs spec drift"
echo ""

echo "🔧 Supported outputs:"
echo "  📚 Interactive HTML documentation (REST + gRPC)"
echo "  🔧 Multi-language SDKs (Go, Python, TypeScript, Java)"
echo "  📦 Protobuf files generated from OpenAPI"
echo "  📊 Drift detection reports"
echo ""

echo "🎯 Perfect for:"
echo "  • API-first development teams"
echo "  • Microservices architecture"
echo "  • Multi-language API ecosystems"
echo "  • CI/CD pipeline integration"
echo "  • API governance and compliance"
echo ""

echo "📝 Example workflow:"
echo "  1. specguard scan ./my-api"
echo "  2. specguard drift ./my-api --compare-with=staging"
echo "  3. specguard generate ./my-api --output=./artifacts --formats=docs,sdk,proto"
echo ""
