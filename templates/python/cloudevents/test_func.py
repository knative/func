import json
import unittest

func = __import__("func")

class TestFunc(unittest.TestCase):

  def test_func(self):
    body = func.main({})
    self.assertEqual(body.data["message"],  "Howdy!")

if __name__ == "__main__":
  unittest.main()