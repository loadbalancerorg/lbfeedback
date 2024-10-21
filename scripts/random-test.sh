#!/bin/bash

lowerRange=0   # inclusive
upperRange=100   # exclusive

randomNumber=$(( RANDOM * ( upperRange - lowerRange) / 32767 + lowerRange ))
echo $randomNumber
exit 0;
