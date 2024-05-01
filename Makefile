shell:
	flyctl postgres connect -a pcomdb

tunnel:
	flyctl proxy 5432 -a pcomdb
