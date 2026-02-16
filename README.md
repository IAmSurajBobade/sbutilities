# scripts
Collection of all the scripts created so far


## Usage
```bash
# Aadhar util
# Input: PDF file or folder path
# Output: PDF file path (optional, default: <input>.pdf)
# Search: Process only files containing this string (folder mode only)
go run ./cmd/utils/main.go aadhar --in data/aadhar-util/input-files --out data/aadhar-util/output --search Aadhar
```