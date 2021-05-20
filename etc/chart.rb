require 'json'
require 'erb'

class Measurment
  class Point < Struct.new(:clients, :max, :min, :median, :p95, keyword_init: true)
  end

  class Step
    include Comparable

    attr_reader :num, :points

    def initialize(num)
      @num = num
      @points = []
    end

    def <<(point)
      points << point
    end

    def <=>(other)
      num <=> other.num
    end

    def max
      mean(:max)
    end

    def p95
      mean(:p95)
    end

    def median
      mean(:median)
    end

    private

    def mean(field)
      sum = points.sum { _1[field].to_f }

      sum / points.size
    end
  end

  attr_reader :type, :steps

  def initialize(type)
    @type = type
    @steps = Hash.new { |h, k| h[k] = Step.new(k) }
  end

  def <<(point_data)
    point = Point.new(
      clients: point_data["clients"],
      max: point_data["max-rtt"],
      min: point_data["min-rtt"],
      median: point_data["median-rtt"],
      p95: point_data["per-rtt"]
    )
    steps[point.clients] << point
  end
end

data = Hash.new { |h,k| h[k] = Measurment.new(k) }

target_dir = ARGV[1] || File.join(__dir__, "../dist")

Dir.glob(File.join(target_dir, "*.json")).each do |report|
  matches = report.match(/\/([^\/]+)\_\d+.json/)
  next puts "Unknown file: #{report}" unless matches

  report_type = matches[1]
  report_data = JSON.parse(File.read(report))

  report_data["steps"].each do |step|
    data[report_type] << step
  end
end

series = {}
%i[median p95 max].each do |field|
  data.map do |k, v|
    "{\n" +
    "name: '#{k}',\n" +
    "data: #{v.steps.values.map(&field)}\n" +
    "}\n"
  end.join(",\n").then { "[#{_1}]\n" }.then { series[field] = _1 }
end

renderer = ERB.new(DATA.read)
puts renderer.result(binding)

__END__

<!DOCTYPE html>
<html>
  <head>
    <title>WebSocket bench results</title>
    <meta name="viewport" content="width=device-width">
    <meta charset="UTF-8">
    <script src="https://code.highcharts.com/highcharts.js"></script>
    <script src="https://code.highcharts.com/modules/series-label.js"></script>
    <script src="https://code.highcharts.com/modules/exporting.js"></script>
    <script src="https://code.highcharts.com/modules/export-data.js"></script>
    <script src="https://code.highcharts.com/modules/accessibility.js"></script>
    <style>
      .highcharts-figure, .highcharts-data-table table {
        min-width: 360px;
        max-width: 800px;
        margin: 1em auto;
      }

      .highcharts-data-table table {
        font-family: Verdana, sans-serif;
        border-collapse: collapse;
        border: 1px solid #EBEBEB;
        margin: 10px auto;
        text-align: center;
        width: 100%;
        max-width: 500px;
      }

      .highcharts-data-table caption {
        padding: 1em 0;
        font-size: 1.2em;
        color: #555;
      }
      .highcharts-data-table th {
        font-weight: 600;
        padding: 0.5em;
      }

      .highcharts-data-table td, .highcharts-data-table th, .highcharts-data-table caption {
        padding: 0.5em;
      }

      .highcharts-data-table thead tr, .highcharts-data-table tr:nth-child(even) {
        background: #f8f8f8;
      }

      .highcharts-data-table tr:hover {
        background: #f1f7ff;
      }
    </style>
  </head>
  <body>
    <figure class="highcharts-figure">
      <div id="container--mean"></div>
      <div id="container--95p"></div>
      <div id="container--max"></div>
    </figure>

    <script>
    function drawChart(id, title, series) {
      Highcharts.chart(id, {
        title: {
          text: title
        },
        yAxis: {
          title: {
            text: 'ms'
          }
        },
        xAxis: {
          title: {
            text: 'number of clients (thousands)'
          }
        },
        legend: {
          layout: 'vertical',
          align: 'right',
          verticalAlign: 'middle'
        },
        plotOptions: {
          series: {
            label: {
              connectorAllowed: false
            },
            pointStart: 0
          }
        },
        series: series,
        responsive: {
          rules: [{
            condition: {
              maxWidth: 500
            },
            chartOptions: {
              legend: {
                layout: 'horizontal',
                align: 'center',
                verticalAlign: 'bottom'
              }
            }
        }]
      }
    })};

    drawChart('container--mean', 'Median RTT (msg) with different encoders',
      <%= series[:median] %>
    )

    drawChart('container--95p', '95p RTT (msg) with different encoders',
      <%= series[:p95] %>
    )

    drawChart('container--max', 'Max RTT (msg) with different encoders',
      <%= series[:max] %>
    )
    </script>
  </body>
</html>
