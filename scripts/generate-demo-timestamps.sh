#!/bin/bash

# Demo script for generating timestamps with relative time expressions
# Usage: ./generate-demo-timestamps.sh [format] [count]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_example() {
    echo -e "${YELLOW}[EXAMPLE]${NC} $1"
}

# Function to generate relative timestamps using Python
generate_relative_timestamps() {
    local expressions=("$@")
    
    python3 -c "
import datetime
import re
import sys

def parse_relative_time(relative_str):
    relative_str = relative_str.strip().lower()
    
    if relative_str == 'now':
        return datetime.datetime.now(datetime.timezone.utc)
    
    # Parse expressions like 'now-5m', 'now+30s', etc.
    match = re.match(r'^now\s*([+-])\s*(\d+)\s*([smhd])$', relative_str)
    if not match:
        raise ValueError(f'Invalid format: {relative_str}')
    
    sign, value_str, unit = match.groups()
    value = int(value_str)
    
    # Convert to timedelta
    if unit == 's':
        delta = datetime.timedelta(seconds=value)
    elif unit == 'm':
        delta = datetime.timedelta(minutes=value)
    elif unit == 'h':
        delta = datetime.timedelta(hours=value)
    elif unit == 'd':
        delta = datetime.timedelta(days=value)
    else:
        raise ValueError(f'Invalid unit: {unit}')
    
    now = datetime.datetime.now(datetime.timezone.utc)
    if sign == '-':
        return now - delta
    else:
        return now + delta

# Process each expression
expressions = sys.argv[1:]
for expr in expressions:
    try:
        timestamp = parse_relative_time(expr)
        print(f'{expr:12} -> {timestamp.strftime(\"%Y-%m-%dT%H:%M:%SZ\")}')
    except ValueError as e:
        print(f'{expr:12} -> ERROR: {e}')
" "${expressions[@]}"
}

# Function to generate CSV with relative timestamps
generate_demo_csv_with_relative_times() {
    local filename="$1"
    local start_relative="${2:-now-1h}"
    local end_relative="${3:-now}"
    local count="${4:-20}"
    
    print_status "Generating CSV with timestamps from $start_relative to $end_relative ($count records)"
    
    python3 -c "
import datetime
import re
import random
import sys

def parse_relative_time(relative_str):
    relative_str = relative_str.strip().lower()
    
    if relative_str == 'now':
        return datetime.datetime.now(datetime.timezone.utc)
    
    match = re.match(r'^now\s*([+-])\s*(\d+)\s*([smhd])$', relative_str)
    if not match:
        raise ValueError(f'Invalid format: {relative_str}')
    
    sign, value_str, unit = match.groups()
    value = int(value_str)
    
    if unit == 's':
        delta = datetime.timedelta(seconds=value)
    elif unit == 'm':
        delta = datetime.timedelta(minutes=value)
    elif unit == 'h':
        delta = datetime.timedelta(hours=value)
    elif unit == 'd':
        delta = datetime.timedelta(days=value)
    else:
        raise ValueError(f'Invalid unit: {unit}')
    
    now = datetime.datetime.now(datetime.timezone.utc)
    if sign == '-':
        return now - delta
    else:
        return now + delta

filename = sys.argv[1]
start_relative = sys.argv[2]
end_relative = sys.argv[3]
count = int(sys.argv[4])

try:
    start_time = parse_relative_time(start_relative)
    end_time = parse_relative_time(end_relative)
    
    if start_time > end_time:
        start_time, end_time = end_time, start_time
    
    duration = end_time - start_time
    interval = duration / (count - 1) if count > 1 else datetime.timedelta(0)
    
    with open(filename, 'w') as f:
        # Write header
        f.write('timestamp,gpu_id,hostname,uuid,device,modelName,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz\\n')
        
        # Generate records
        for i in range(count):
            timestamp = start_time + (interval * i)
            
            # Add some random jitter (±30 seconds)
            jitter = datetime.timedelta(seconds=random.randint(-30, 30))
            timestamp += jitter
            
            gpu_id = i % 4
            hostname = f'demo-host-{(i//4)+1:03d}'
            uuid = f'GPU-{i:08d}-1234-5678-9abc-{i:012d}'
            device = f'nvidia{gpu_id}'
            model = 'NVIDIA H100 80GB HBM3'
            gpu_util = round(70 + random.random() * 30, 1)
            mem_util = round(60 + random.random() * 30, 1)
            mem_used = 50000 + random.randint(0, 30000)
            mem_free = 10000 + random.randint(0, 20000)
            temp = round(55 + random.random() * 20, 1)
            power = round(300 + random.random() * 150, 1)
            sm_clock = 1400 + random.randint(0, 200)
            mem_clock = 1200 + random.randint(0, 100)
            
            f.write(f'{timestamp.strftime(\"%Y-%m-%dT%H:%M:%SZ\")},{gpu_id},{hostname},{uuid},{device},{model},{gpu_util},{mem_util},{mem_used},{mem_free},{temp},{power},{sm_clock},{mem_clock}\\n')
    
    print(f'Generated {filename} with {count} records from {start_time.strftime(\"%Y-%m-%dT%H:%M:%SZ\")} to {end_time.strftime(\"%Y-%m-%dT%H:%M:%SZ\")}')
    
except Exception as e:
    print(f'Error: {e}')
    sys.exit(1)
" "$filename" "$start_relative" "$end_relative" "$count"
}

# Function to show examples
show_examples() {
    echo "🕐 Relative Timestamp Examples"
    echo "=============================="
    echo ""
    
    print_status "Basic relative time expressions:"
    generate_relative_timestamps "now" "now-5s" "now-30s" "now-5m" "now-15m" "now-1h" "now-2h" "now-1d"
    
    echo ""
    print_status "More complex examples:"
    generate_relative_timestamps "now-90s" "now-45m" "now-6h" "now-3d"
    
    echo ""
    print_status "Future timestamps (useful for testing):"
    generate_relative_timestamps "now+5s" "now+1m" "now+1h"
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  examples                           - Show timestamp examples"
    echo "  generate <expressions...>          - Generate specific timestamps"
    echo "  csv <file> <start> <end> <count>  - Generate CSV with timestamp range"
    echo "  help                              - Show this help"
    echo ""
    echo "Relative Time Format:"
    echo "  now      - Current time"
    echo "  now-5s   - 5 seconds ago"
    echo "  now-5m   - 5 minutes ago"
    echo "  now-5h   - 5 hours ago"
    echo "  now-5d   - 5 days ago"
    echo "  now+5m   - 5 minutes from now"
    echo ""
    echo "Examples:"
    echo "  $0 examples"
    echo "  $0 generate now-5m now-1m now"
    echo "  $0 csv demo.csv now-2h now 100"
}

# Function to demonstrate common patterns
show_common_patterns() {
    echo "📊 Common Demo Patterns"
    echo "======================"
    echo ""
    
    print_example "Recent activity (last 5 minutes):"
    generate_relative_timestamps "now-5m" "now-4m" "now-3m" "now-2m" "now-1m" "now"
    
    echo ""
    print_example "Hourly snapshots (last 6 hours):"
    generate_relative_timestamps "now-6h" "now-5h" "now-4h" "now-3h" "now-2h" "now-1h" "now"
    
    echo ""
    print_example "Daily snapshots (last week):"
    generate_relative_timestamps "now-7d" "now-6d" "now-5d" "now-4d" "now-3d" "now-2d" "now-1d" "now"
}

# Main execution
main() {
    case "${1:-examples}" in
        "examples")
            show_examples
            echo ""
            show_common_patterns
            ;;
        "generate")
            shift
            if [ $# -eq 0 ]; then
                print_status "No expressions provided. Showing examples:"
                show_examples
            else
                print_status "Generating timestamps for: $*"
                generate_relative_timestamps "$@"
            fi
            ;;
        "csv")
            filename="${2:-demo_timestamps.csv}"
            start_relative="${3:-now-1h}"
            end_relative="${4:-now}"
            count="${5:-50}"
            generate_demo_csv_with_relative_times "$filename" "$start_relative" "$end_relative" "$count"
            print_success "CSV file generated: $filename"
            ;;
        "patterns")
            show_common_patterns
            ;;
        "help"|"-h"|"--help")
            show_usage
            ;;
        *)
            echo "Unknown command: $1"
            echo ""
            show_usage
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
