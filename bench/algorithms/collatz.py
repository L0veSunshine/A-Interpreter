"""Project Euler 14: 最长 Collatz 链 —— 纯整数标量循环，衡量分支与取模/整除。"""


def chain_len(n):
    steps = 1
    while n != 1:
        if n % 2 == 0:
            n = n // 2
        else:
            n = 3 * n + 1
        steps += 1
    return steps


best = 0
best_n = 0
for i in range(1, 50000):
    s = chain_len(i)
    if s > best:
        best = s
        best_n = i
print([best_n, best])
