<?php
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# source: graphik.proto

namespace Api;

use Google\Protobuf\Internal\GPBType;
use Google\Protobuf\Internal\RepeatedField;
use Google\Protobuf\Internal\GPBUtil;

/**
 * DocConstructor is used to create a batch of docs
 *
 * Generated from protobuf message <code>api.DocConstructors</code>
 */
class DocConstructors extends \Google\Protobuf\Internal\Message
{
    /**
     * docs is an array of doc constructors
     *
     * Generated from protobuf field <code>repeated .api.DocConstructor docs = 1;</code>
     */
    private $docs;

    /**
     * Constructor.
     *
     * @param array $data {
     *     Optional. Data for populating the Message object.
     *
     *     @type \Api\DocConstructor[]|\Google\Protobuf\Internal\RepeatedField $docs
     *           docs is an array of doc constructors
     * }
     */
    public function __construct($data = NULL) {
        \GPBMetadata\Graphik::initOnce();
        parent::__construct($data);
    }

    /**
     * docs is an array of doc constructors
     *
     * Generated from protobuf field <code>repeated .api.DocConstructor docs = 1;</code>
     * @return \Google\Protobuf\Internal\RepeatedField
     */
    public function getDocs()
    {
        return $this->docs;
    }

    /**
     * docs is an array of doc constructors
     *
     * Generated from protobuf field <code>repeated .api.DocConstructor docs = 1;</code>
     * @param \Api\DocConstructor[]|\Google\Protobuf\Internal\RepeatedField $var
     * @return $this
     */
    public function setDocs($var)
    {
        $arr = GPBUtil::checkRepeatedField($var, \Google\Protobuf\Internal\GPBType::MESSAGE, \Api\DocConstructor::class);
        $this->docs = $arr;

        return $this;
    }

}

