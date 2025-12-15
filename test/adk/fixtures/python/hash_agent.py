"""Hash agent for testing schema hashing"""
import sys
import hashlib

ID = "hash_agent"
TYPE = "service"
VERSION = "1.0.0"
DESCRIPTION = "Agent for testing schema hashing"
CAPABILITIES = ["compute_schema_hash"]


def compute_hash(filename):
    """Compute BLAKE3-compatible hash (using SHA256 as fallback for testing)"""
    try:
        with open(filename, 'rb') as f:
            # In production, this would use BLAKE3
            # For testing, we use SHA256 which also produces 64-char hex
            h = hashlib.sha256()
            h.update(f.read())
            return h.hexdigest()
    except Exception as e:
        print(f"Failed to hash file: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: hash_agent.py <filename>", file=sys.stderr)
        sys.exit(1)
    
    filename = sys.argv[1]
    print(compute_hash(filename))
