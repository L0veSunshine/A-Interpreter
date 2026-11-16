"""LeetCode 1143: Longest Common Subsequence —— 二维 DP 展平成一维数组。"""


def lcs(s, t):
    n = len(s)
    m = len(t)
    width = m + 1
    dp = [0] * ((n + 1) * width)
    for i in range(1, n + 1):
        base = i * width
        prev = base - width
        for j in range(1, m + 1):
            if s[i - 1] == t[j - 1]:
                dp[base + j] = dp[prev + j - 1] + 1
            else:
                up = dp[prev + j]
                left = dp[base + j - 1]
                dp[base + j] = up if up > left else left
    return dp[n * width + m]


s = "abcdefghij"
t = "jihgfedcbazyxwv"
for _ in range(6):
    s = s + s
    t = t + t

print(lcs(s, t))
