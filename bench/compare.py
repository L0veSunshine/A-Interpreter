#!/usr/bin/env python3
"""xlang 与 CPython 的算法对比：同一批算法、同样的输入、同样的输出。

用法：
    python bench/compare.py             # 每个程序跑 5 次，取最好成绩
    python bench/compare.py --runs 9
    python bench/compare.py --no-build  # 复用已经构建好的 bench/.bin/xlang

每个算法在 bench/algorithms 下有配对的 <name>.xl / <name>.py：
算法步骤、数据规模、打印的结果完全一致，因此耗时可以直接对比。
报告的是端到端墙钟时间（含解释器启动），启动开销单独列在最下面。
"""

import argparse
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ALGO_DIR = ROOT / "bench" / "algorithms"
BIN_DIR = ROOT / "bench" / ".bin"
XLANG = BIN_DIR / ("xlang.exe" if sys.platform == "win32" else "xlang")


def build_xlang() -> None:
    BIN_DIR.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        ["go", "build", "-o", str(XLANG), "./cmd/xlang"],
        cwd=ROOT,
        check=True,
    )


def run_once(cmd: list[str]) -> tuple[float, str]:
    start = time.perf_counter()
    proc = subprocess.run(cmd, capture_output=True, text=True)
    elapsed = time.perf_counter() - start
    if proc.returncode != 0:
        raise RuntimeError(
            f"command failed: {' '.join(cmd)}\n{proc.stdout}{proc.stderr}"
        )
    lines = proc.stdout.strip().splitlines()
    return elapsed, lines[-1] if lines else ""


def best_of(cmd: list[str], runs: int) -> tuple[float, str]:
    best = None
    output = ""
    for _ in range(runs):
        elapsed, output = run_once(cmd)
        best = elapsed if best is None else min(best, elapsed)
    return best, output


def measure_startup(runs: int) -> tuple[float, float]:
    BIN_DIR.mkdir(parents=True, exist_ok=True)
    probe = BIN_DIR / "_startup.xl"
    probe.write_text("0\n", encoding="utf-8")
    xlang_ms = best_of([str(XLANG), str(probe)], runs)[0] * 1000
    python_ms = best_of([sys.executable, "-c", "pass"], runs)[0] * 1000
    return xlang_ms, python_ms


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--runs", type=int, default=5, help="每个程序运行次数（取最好成绩）")
    parser.add_argument("--no-build", action="store_true", help="跳过 go build")
    args = parser.parse_args()

    if not args.no_build:
        build_xlang()
    if not XLANG.exists():
        raise SystemExit(f"缺少 {XLANG}，先运行 go build -o {XLANG} ./cmd/xlang")

    names = sorted(p.stem for p in ALGO_DIR.glob("*.xl"))
    if not names:
        raise SystemExit(f"{ALGO_DIR} 下没有找到 .xl 程序")

    print(f"Python {sys.version.split()[0]}  /  runs = {args.runs} (best of)\n")
    print(f"{'algorithm':<12}{'xlang (ms)':>12}{'Python (ms)':>14}{'xlang/Python':>14}   output")
    print("-" * 70)

    total_x = total_p = 0.0
    for name in names:
        xl_out = ALGO_DIR / f"{name}.xl"
        py_out = ALGO_DIR / f"{name}.py"
        if not py_out.exists():
            raise SystemExit(f"{name}.xl 缺少配对的 {py_out.name}")

        xl_ms, xl_result = best_of([str(XLANG), str(xl_out)], args.runs)
        py_ms, py_result = best_of([sys.executable, str(py_out)], args.runs)
        xl_ms *= 1000
        py_ms *= 1000
        total_x += xl_ms
        total_p += py_ms

        flag = "" if xl_result == py_result else "  <-- RESULT MISMATCH"
        print(
            f"{name:<12}{xl_ms:>12.1f}{py_ms:>14.1f}{xl_ms / py_ms:>13.2f}x"
            f"   {xl_result}{flag}"
        )
        if xl_result != py_result:
            print(f"    xlang:  {xl_result}\n    python: {py_result}")
            return 1

    print("-" * 70)
    print(f"{'total':<12}{total_x:>12.1f}{total_p:>14.1f}{total_x / total_p:>13.2f}x")

    xl_start, py_start = measure_startup(args.runs)
    print(f"\nstartup (empty program): xlang {xl_start:.1f} ms / Python {py_start:.1f} ms")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
