"""LeetCode 1: Two Sum —— 哈希表。"""


def two_sum(a, target):
    seen = {}
    for i in range(len(a)):
        need = target - a[i]
        got = seen.get(need)
        if got is not None:
            return [got, i]
        seen[a[i]] = i
    return [-1, -1]


n = 30000
a = []
seed = 13579
for _ in range(n):
    seed = (seed * 1103515245 + 12345) % 2147483648
    a.append(seed % 1000000)

total = 0
for r in range(80):
    target = a[r] + a[n - 1 - r]
    pair = two_sum(a, target)
    total += pair[0] + pair[1]

print(total)
