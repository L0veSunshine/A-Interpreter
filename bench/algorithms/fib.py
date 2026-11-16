"""LeetCode 509: Fibonacci Number —— 朴素递归，衡量调用/建帧开销。"""


def fib(n):
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)


print(fib(30))
