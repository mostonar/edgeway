
docker pull quay.io/jupyter/base-notebook:latest

cd ~/etsi-mec-sandbox/examples/demo6/python/ && docker run --rm -it -d --expose 31111 -p 31111:31111 -p 9999:8888 -v"$PWD:/home/jovyan/work" quay.io/jupyter/base-notebook:latest

curl --verbose --request GET http://mec-platform2.etsi.org:31111/sandbox/v1/statistic/v1/quantity --header "Accept: application/json" --data '{"time":20180124,"data1":"[1516752000,11590.6,11616.9,11590.4,11616.9,0.25202387,1516752060,11622.4,11651.7,11622.4,11644.6,1.03977764]"}'
