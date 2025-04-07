# Deployment Guide

This guide covers how to deploy NannyAPI in various environments.

## Docker Deployment

### Prerequisites
- Docker Engine 20.10+
- Docker Compose 2.0+
- Access to MongoDB instance
- SSL certificates (for production)

### Steps

1. **Build the Docker image:**
```bash
docker build -t nannyapi:latest .
```

2. **Configure environment:**
Create a `.env` file with production settings:
```env
GIN_MODE=release
MONGODB_URI=your-production-mongodb-uri
NANNY_ENCRYPTION_KEY=your-production-key
GH_CLIENT_ID=your-github-client-id
GH_CLIENT_SECRET=your-github-client-secret
```

3. **Deploy with Docker Compose:**
```bash
docker-compose up -d
```

## Kubernetes Deployment

### Prerequisites
- Kubernetes cluster 1.24+
- kubectl configured
- Helm 3.0+

### Steps

1. **Add Helm repository:**
```bash
helm repo add nannyapi https://charts.nannyai.dev
helm repo update
```

2. **Create values file:**
```yaml
# values.yaml
replicaCount: 3

image:
  repository: nannyapi
  tag: latest
  pullPolicy: Always

mongodb:
  uri: mongodb://your-mongodb-service:27017

secrets:
  encryptionKey: your-base64-encoded-key
  githubClientId: your-base64-encoded-id
  githubClientSecret: your-base64-encoded-secret

ingress:
  enabled: true
  host: api.nannyai.dev
  tls: true
```

3. **Deploy:**
```bash
helm install nannyapi nannyapi/nannyapi -f values.yaml
```

## Cloud Provider Deployments

### AWS ECS

1. **Create ECR repository:**
```bash
aws ecr create-repository --repository-name nannyapi
```

2. **Push image:**
```bash
aws ecr get-login-password --region region | docker login --username AWS --password-stdin aws_account_id.dkr.ecr.region.amazonaws.com
docker tag nannyapi:latest aws_account_id.dkr.ecr.region.amazonaws.com/nannyapi:latest
docker push aws_account_id.dkr.ecr.region.amazonaws.com/nannyapi:latest
```

3. **Create ECS Task Definition and Service**
   - Use AWS Console or CloudFormation
   - Configure environment variables
   - Set up load balancer
   - Enable auto-scaling

### Google Cloud Run

1. **Build and push:**
```bash
gcloud builds submit --tag gcr.io/project-id/nannyapi
```

2. **Deploy:**
```bash
gcloud run deploy nannyapi \
  --image gcr.io/project-id/nannyapi \
  --platform managed \
  --region us-central1 \
  --set-env-vars="MONGODB_URI=your-uri"
```

## Monitoring and Logging

### Prometheus Metrics
The application exposes metrics at `/metrics`:
- Request latency
- Error rates
- MongoDB operation metrics
- Custom business metrics

### Logging
Logs are written in JSON format to stdout:
```json
{
  "level": "info",
  "timestamp": "2025-04-06T12:00:00Z",
  "caller": "server/server.go:42",
  "msg": "Request processed",
  "method": "GET",
  "path": "/api/status",
  "duration_ms": 5
}
```

### Health Checks
- Readiness: `/health/ready`
- Liveness: `/health/live`

## SSL/TLS Configuration

1. **Generate certificates:**
```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout nannyapi.key -out nannyapi.crt
```

2. **Configure in environment:**
```env
TLS_CERT_PATH=/path/to/nannyapi.crt
TLS_KEY_PATH=/path/to/nannyapi.key
```

## Database Management

### Backup Strategy
1. **Regular backups:**
```bash
mongodump --uri="mongodb://your-uri" --out=/backup/$(date +%Y%m%d)
```

2. **Restore if needed:**
```bash
mongorestore --uri="mongodb://your-uri" /backup/20250406
```

## Scaling Guidelines

### Horizontal Scaling
- Use container orchestration (Kubernetes/ECS)
- Enable auto-scaling based on CPU/memory
- Configure proper readiness/liveness probes

### Vertical Scaling
- Monitor resource usage
- Adjust container resources as needed
- Scale MongoDB appropriately

## Troubleshooting

### Common Issues

1. **Connection Timeouts**
   - Check network policies
   - Verify MongoDB connection string
   - Check DNS resolution

2. **High Memory Usage**
   - Review goroutine leaks
   - Check MongoDB connection pool
   - Monitor memory metrics

3. **High Latency**
   - Check database indices
   - Review slow queries
   - Monitor network latency

## Rollback Procedures

1. **Container-based:**
```bash
docker-compose up -d --rollback
```

2. **Kubernetes:**
```bash
kubectl rollout undo deployment/nannyapi
```

## Security Considerations

1. **Network Security**
   - Use VPC/private networks
   - Implement proper firewalls
   - Enable TLS everywhere

2. **Access Control**
   - Use IAM roles
   - Implement RBAC
   - Regular audit logging

3. **Secrets Management**
   - Use secrets management service
   - Rotate credentials regularly
   - Monitor for exposures

## Maintenance

### Regular Tasks
- Update dependencies monthly
- Rotate credentials quarterly
- Review and update SSL certificates
- Monitor disk usage
- Check backup integrity

### Performance Optimization
- Review and update indices
- Monitor query performance
- Optimize caching strategy
- Review resource allocation
