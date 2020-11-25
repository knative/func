package functions;

public class Output {

  public String input;
  public String operation;
  public String output;
  public String error;

  @Override
  public String toString() {
    return "Output{" +
        "input='" + input + '\'' +
        ", operation='" + operation + '\'' +
        ", output='" + output + '\'' +
        ", error='" + error + '\'' +
        '}';
  }
}
