#!/bin/bash

# Windshift Docker deployment script
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_usage() {
    echo "Usage: $0 [build|start|stop|restart|logs|status|clean]"
    echo ""
    echo "Commands:"
    echo "  build    - Build the Docker image from source"
    echo "  quick    - Build using pre-built binary (faster)"
    echo "  start    - Start the container in background"
    echo "  stop     - Stop the container"
    echo "  restart  - Restart the container"
    echo "  logs     - Show container logs"
    echo "  status   - Show container status"
    echo "  clean    - Remove container and volumes (WARNING: deletes data)"
    echo ""
    echo "Examples:"
    echo "  $0 build   # Build from source"
    echo "  $0 quick   # Build with pre-built binary"
    echo "  $0 start   # Start the server"
}

case "$1" in
    build)
        echo -e "${BLUE}Building Windshift Docker image from source...${NC}"
        docker build -t windshift:latest .
        echo -e "${GREEN}✅ Image built successfully${NC}"
        ;;
        
    quick)
        echo -e "${BLUE}Building Windshift Docker image with pre-built binary...${NC}"
        # First ensure binary is built
        if [ ! -f "dist/server/windshift-linux-amd64" ]; then
            echo -e "${YELLOW}Binary not found, building it first...${NC}"
            ./build-all.sh
        fi
        docker build -f Dockerfile.prebuilt -t windshift:latest .
        echo -e "${GREEN}✅ Image built successfully${NC}"
        ;;
        
    start)
        echo -e "${BLUE}Starting Windshift container...${NC}"
        docker run -d \
            --name windshift-server \
            -p 2222:8080 \
            -v windshift-data:/data \
            -v windshift-attachments:/data/attachments \
            --restart unless-stopped \
            windshift:latest \
            -p 8080 \
            -db /data/windshift.db \
            --attachment-path /data/attachments \
            --allowed-hosts "lihue,lihue.network.realigned,192.168.1.30,localhost" \
            --allowed-port 2222
        echo -e "${GREEN}✅ Windshift server started on port 2222${NC}"
        echo "Access at: http://localhost:2222"
        ;;
        
    stop)
        echo -e "${YELLOW}Stopping Windshift container...${NC}"
        docker stop windshift-server
        docker rm windshift-server
        echo -e "${GREEN}✅ Container stopped${NC}"
        ;;
        
    restart)
        echo -e "${YELLOW}Restarting Windshift container...${NC}"
        docker restart windshift-server
        echo -e "${GREEN}✅ Container restarted${NC}"
        ;;
        
    logs)
        docker logs -f windshift-server
        ;;
        
    status)
        echo -e "${BLUE}Windshift container status:${NC}"
        docker ps -a | grep windshift-server || echo "Container not found"
        echo ""
        echo -e "${BLUE}Windshift volumes:${NC}"
        docker volume ls | grep windshift
        ;;
        
    clean)
        echo -e "${YELLOW}⚠️  WARNING: This will delete all data!${NC}"
        read -p "Are you sure? (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            docker stop windshift-server 2>/dev/null || true
            docker rm windshift-server 2>/dev/null || true
            docker volume rm windshift-data windshift-attachments 2>/dev/null || true
            echo -e "${GREEN}✅ Cleaned up container and volumes${NC}"
        else
            echo "Cancelled"
        fi
        ;;
        
    *)
        print_usage
        exit 1
        ;;
esac