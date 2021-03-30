import json
import unittest

func = __import__("func")

class TestFunc(unittest.TestCase):

  def test_func(self):
    body, code, _ = func.main({})
    resp = json.loads(body)
    self.assertEqual(resp["message"],  "Howdy!")
    self.assertEqual(code, 200)

if __name__ == "__main__":
  unittest.main()