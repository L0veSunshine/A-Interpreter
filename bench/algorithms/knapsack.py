"""0/1 背包：经典二维 DP 展平成一维数组。"""


def knapsack(weights, values, cap):
    n = len(weights)
    width = cap + 1
    dp = [0] * ((n + 1) * width)
    for i in range(1, n + 1):
        wi = weights[i - 1]
        vi = values[i - 1]
        base = i * width
        prev = base - width
        for c in range(cap + 1):
            best = dp[prev + c]
            if wi <= c:
                cand = dp[prev + c - wi] + vi
                if cand > best:
                    best = cand
            dp[base + c] = best
    return dp[n * width + cap]


n = 800
cap = 2000
weights = []
values = []
seed = 24680
for _ in range(n):
    seed = (seed * 1103515245 + 12345) % 2147483648
    weights.append(seed % 50 + 1)
    seed = (seed * 1103515245 + 12345) % 2147483648
    values.append(seed % 100 + 1)

print(knapsack(weights, values, cap))
