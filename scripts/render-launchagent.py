#!/usr/bin/env python3
"""Render a per-user macOS ReaderHub LaunchAgent; this does not install it."""
import argparse
from pathlib import Path
import plistlib

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--binary', required=True)
parser.add_argument('--data', required=True)
parser.add_argument('--listen', required=True)
parser.add_argument('--output', type=Path, required=True)
args = parser.parse_args()
for value in (args.binary, args.data):
    if not Path(value).is_absolute():
        parser.error('binary and data paths must be absolute on the target Mac')
agent = {
    'Label': 'io.readerhub.hub',
    'ProgramArguments': [args.binary, 'serve', '--listen', args.listen, '--data', args.data],
    'WorkingDirectory': args.data,
    'RunAtLoad': True,
    'KeepAlive': True,
    'ThrottleInterval': 10,
    'StandardOutPath': str(Path(args.data) / 'logs/stdout.log'),
    'StandardErrorPath': str(Path(args.data) / 'logs/stderr.log'),
}
args.output.parent.mkdir(parents=True, exist_ok=True)
with args.output.open('wb') as file:
    plistlib.dump(agent, file)
print(f'Rendered {args.output}; no service installed')
