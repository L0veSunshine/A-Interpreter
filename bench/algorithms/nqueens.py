"""LeetCode 52: N-Queens II —— 回溯 + 递归。"""


def nqueens(n, row, cols, diag1, diag2):
    if row == n:
        return 1
    total = 0
    for c in range(n):
        if cols[c] == 0 and diag1[row + c] == 0 and diag2[row - c + n] == 0:
            cols[c] = 1
            diag1[row + c] = 1
            diag2[row - c + n] = 1
            total += nqueens(n, row + 1, cols, diag1, diag2)
            cols[c] = 0
            diag1[row + c] = 0
            diag2[row - c + n] = 0
    return total


n = 11
cols = [0] * n
diag1 = [0] * (2 * n + 1)
diag2 = [0] * (2 * n + 1)

print(nqueens(n, 0, cols, diag1, diag2))
