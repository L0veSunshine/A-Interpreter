"""LeetCode 704: Binary Search —— 有序数组上的大量下界二分，衡量整数除法与分支。"""

n = 300000
a = [3 * i for i in range(n)]


def lower_bound(a, target):
    lo, hi = 0, len(a)
    while lo < hi:
        mid = (lo + hi) // 2
        if a[mid] < target:
            lo = mid + 1
        else:
            hi = mid
    return lo


seed = 4242
hits = 0
total = 0
for _ in range(400000):
    seed = (seed * 1103515245 + 12345) % 2147483648
    t = seed % (3 * n)
    p = lower_bound(a, t)
    if p < n and a[p] == t:
        hits += 1
    total += p
print([hits, total])
